/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package debug_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/cue/cuex/providers/debug"
)

// workload is a definition of the size one actually is, so the calls are
// measured against a value worth walking rather than against nothing.
const workload = `
parameter: {
	image:   string
	cpu?:    string
	replicas?: *1 | int
	env?: [...{name: string, value?: string}]
	ports?: [...{port: int, protocol: *"TCP" | "UDP"}]
	labels?: [string]: string
}
context: {name: "my-app", namespace: "prod"}
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {name: context.name, namespace: context.namespace}
	spec: {
		replicas: parameter.replicas
		selector: matchLabels: "app.oam.dev/component": context.name
		template: spec: containers: [{
			name:  context.name
			image: parameter.image
			if parameter.env != _|_ {env: parameter.env}
			if parameter.ports != _|_ {
				ports: [for p in parameter.ports {containerPort: p.port, protocol: p.protocol}]
			}
			if parameter.cpu != _|_ {resources: requests: cpu: parameter.cpu}
		}]
	}
}
parameter: {image: "nginx:1.25", cpu: "500m", replicas: 3}
`

// calling builds the workload with n independent noop calls in it.
func calling(n int) string {
	var b strings.Builder
	if n > 0 {
		b.WriteString("import \"vela/debug\"\n")
	}
	b.WriteString(workload)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\nnoop%d: debug.#Noop & {$params: tag: \"t-%d\"}\n", i, i)
	}
	return b.String()
}

// TestNoopCallsCostWhatTheResolverSpends runs a definition with none, one and
// then a set of calls that do nothing at all, so what is left is the resolver.
// It reports rather than asserts a duration: what it is for is reading, and a
// wall clock threshold would only flake.
func TestNoopCallsCostWhatTheResolverSpends(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	const runs = 50

	t.Log("calls   total      per call   over none")
	var none time.Duration
	for _, n := range []int{0, 1, 2, 4, 8, 16, 32} {
		src := calling(n)
		debug.Reset()

		// once first, so a build that only happens once is not counted
		_, err := c.CompileString(ctx, src)
		require.NoError(t, err)

		start := time.Now()
		for i := 0; i < runs; i++ {
			if _, err := c.CompileString(ctx, src); err != nil {
				t.Fatal(err)
			}
		}
		each := time.Since(start) / runs

		if n == 0 {
			none = each
			t.Logf("%5d %9s %11s %11s", n, each, "-", "-")
			continue
		}
		require.Equal(t, n*(runs+1), debug.Calls(), "every call has to have run")
		t.Logf("%5d %9s %11s %11s", n, each, (each-none)/time.Duration(n), each-none)
	}
}

// TestNoopCallsDoNotChangeTheAnswer: the point of a function that does nothing
// is that it does nothing, so a template that calls it thirty two times has to
// render what the same template renders calling it none.
func TestNoopCallsDoNotChangeTheAnswer(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	plain, err := c.CompileString(ctx, calling(0))
	require.NoError(t, err)
	want, err := plain.LookupPath(outputPath()).MarshalJSON()
	require.NoError(t, err)

	busy, err := c.CompileString(ctx, calling(32))
	require.NoError(t, err)
	got, err := busy.LookupPath(outputPath()).MarshalJSON()
	require.NoError(t, err)

	require.JSONEq(t, string(want), string(got))
}

func outputPath() cue.Path { return cue.ParsePath("output") }
