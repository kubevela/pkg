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

package cuex

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/cue/util"
)

const phaseWorkload = `
parameter: {
	image:   string
	cpu?:    string
	replicas?: *1 | int
	env?: [...{name: string, value?: string}]
	ports?: [...{port: int, protocol: *"TCP" | "UDP"}]
}
context: {name: "my-app", namespace: "prod"}
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {name: context.name, namespace: context.namespace}
	spec: {
		replicas: parameter.replicas
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

func phaseCompiler(tb testing.TB) *Compiler {
	tb.Helper()
	fn := cuexruntime.GenericProviderFn[struct {
		Params string `json:"$params"`
	}, struct {
		Returns string `json:"$returns"`
	}](func(_ context.Context, in *struct {
		Params string `json:"$params"`
	}) (*struct {
		Returns string `json:"$returns"`
	}, error) {
		return &struct {
			Returns string `json:"$returns"`
		}{Returns: in.Params}, nil
	})
	pkg, err := cuexruntime.NewInternalPackage("ph", `
package ph

#Do: {
	#do:       "do"
	#provider: "ph"
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"do": fn})
	require.NoError(tb, err)
	return NewCompilerWithInternalPackages(pkg)
}

func phaseSrc(n int) string {
	var b strings.Builder
	if n > 0 {
		b.WriteString("import \"vela/ph\"\n")
	}
	b.WriteString(phaseWorkload)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\nc%d: ph.#Do & {$params: \"k-%d\"}\n", i, i)
	}
	return b.String()
}

// TestPhases counts the walks and times each phase of a resolve, which is the
// only way to tell a fixed cost from a per call one.
func TestPhases(t *testing.T) {
	c := phaseCompiler(t)
	ctx := context.Background()
	const runs = 200

	t.Log("calls  nodes  walk1  walk2  round   total")
	for _, n := range []int{1, 2, 4, 8, 32} {
		src := phaseSrc(n)
		built, err := c.CompileStringWithOptions(ctx, src, DisableResolveProviderFunctions{})
		require.NoError(t, err)

		var walk1, walk2, round time.Duration
		var nodes int
		start := time.Now()
		for i := 0; i < runs; i++ {
			each, r, n := resolveTimed(t, c, ctx, built)
			walk1 += each[0]
			if len(each) > 1 {
				walk2 += each[1]
			}
			round += r
			nodes = n
		}
		total := time.Since(start) / runs
		t.Logf("%5d %6d %6s %6s %6s %7s", n, nodes,
			(walk1 / runs).Round(time.Microsecond),
			(walk2 / runs).Round(time.Microsecond),
			(round / runs).Round(time.Microsecond),
			total.Round(time.Microsecond))
	}
}

// resolveTimed is resolve, opened up so each phase can be timed separately.
func resolveTimed(t *testing.T, in *Compiler, ctx context.Context, value cue.Value) ([]time.Duration, time.Duration, int) {
	t.Helper()
	newValue := value
	executed := map[string]bool{}
	waitingFor := &callWaits{of: map[string][]string{}}
	providers := in.PackageManager.GetProviders()
	var walkTimes []time.Duration
	var roundTime time.Duration
	nodes := 0
	for {
		w := time.Now()
		pending := pendingCalls(newValue, executed, nil)
		walkTimes = append(walkTimes, time.Since(w))
		if len(pending) == 0 {
			break
		}
		r := time.Now()
		next, opaque, err := in.runRound(ctx, newValue, providers, pending, executed, waitingFor, nil)
		roundTime += time.Since(r)
		require.NoError(t, err)
		newValue = next
		_ = opaque
	}
	util.Iterate(newValue, func(cue.Value) bool { nodes++; return false })
	return walkTimes, roundTime, nodes
}
