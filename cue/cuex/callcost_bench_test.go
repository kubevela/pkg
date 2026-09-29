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

package cuex_test

import (
	"context"
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// realWorkload is a definition of the size one actually is: the cost of a call
// is not the function, it is what the resolver does around it, and that scales
// with the value the call sits in.
const realWorkload = `
parameter: {
	image:   string
	cpu?:    string
	memory?: string
	replicas?: *1 | int
	env?: [...{name: string, value?: string}]
	ports?: [...{port: int, protocol: *"TCP" | "UDP"}]
	labels?: [string]:      string
	annotations?: [string]: string
	volumeMounts?: [...{name: string, mountPath: string}]
}
context: {name: "my-app", namespace: "prod"}

output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {name: context.name, namespace: context.namespace}
	spec: {
		replicas: parameter.replicas
		selector: matchLabels: "app.oam.dev/component": context.name
		template: {
			metadata: labels: "app.oam.dev/component": context.name
			spec: containers: [{
				name:  context.name
				image: parameter.image
				if parameter.env != _|_ {env: parameter.env}
				if parameter.ports != _|_ {
					ports: [for p in parameter.ports {containerPort: p.port, protocol: p.protocol}]
				}
				if parameter.cpu != _|_ {resources: requests: cpu: parameter.cpu}
				if parameter.memory != _|_ {resources: requests: memory: parameter.memory}
				if parameter.volumeMounts != _|_ {volumeMounts: parameter.volumeMounts}
			}]
		}
	}
}
parameter: {image: "nginx:1.25", cpu: "500m", replicas: 3}
`

// nothingParams is the smallest input a provider can take, so what the
// benchmark measures is the resolver and not the function.
type nothingParams struct {
	Params string `json:"$params"`
}

type nothingReturns struct {
	Returns string `json:"$returns"`
}

// nothingCompiler is a provider whose function does nothing at all. Whatever a
// call costs here is what the resolver spends, not what the provider does.
func nothingCompiler(tb testing.TB) *cuex.Compiler {
	tb.Helper()
	fn := cuexruntime.GenericProviderFn[nothingParams, nothingReturns](
		func(_ context.Context, in *nothingParams) (*nothingReturns, error) {
			return &nothingReturns{Returns: in.Params}, nil
		})
	pkg, err := cuexruntime.NewInternalPackage("nothing", `
package nothing

#Do: {
	#do:       "do"
	#provider: "nothing"
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"do": fn})
	require.NoError(tb, err)
	return cuex.NewCompilerWithInternalPackages(pkg)
}

func withCalls(n int) string {
	src := "import \"vela/nothing\"\n" + realWorkload
	for i := 0; i < n; i++ {
		src += fmt.Sprintf("\ncall%d: nothing.#Do & {$params: \"k-%d\"}\n", i, i)
	}
	return src
}

// BenchmarkCallCost is the marginal cost of a call on a value of a real size,
// with a function that does nothing, so the whole of it is the resolver.
func BenchmarkCallCost(b *testing.B) {
	c := nothingCompiler(b)
	ctx := context.Background()
	for _, n := range []int{0, 1, 2, 4, 8} {
		src := withCalls(n)
		b.Run(fmt.Sprintf("calls=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := c.CompileString(ctx, src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkResolveOnly drops the build so what is left is the resolve, which
// is where the marginal cost of a call lives.
func BenchmarkResolveOnly(b *testing.B) {
	c := nothingCompiler(b)
	ctx := context.Background()
	for _, n := range []int{0, 1, 4} {
		src := withCalls(n)
		b.Run(fmt.Sprintf("calls=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				v := buildFor(b, c, src)
				b.StartTimer()
				if _, err := c.Resolve(ctx, v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func buildFor(b *testing.B, c *cuex.Compiler, src string) cue.Value {
	b.Helper()
	v, err := c.CompileStringWithOptions(context.Background(), src, cuex.DisableResolveProviderFunctions{})
	require.NoError(b, err)
	return v
}

// BenchmarkTheTwoHalves splits what a call costs into reading its parameters
// out and putting its answer back, which are the two places a cue.Value is
// touched per call.
func BenchmarkTheTwoHalves(b *testing.B) {
	cc := cuecontext.New()
	root := cc.CompileString(realWorkload + `
call0: {#do: "do", #provider: "nothing", $params: "k-0"}
`)
	require.NoError(b, root.Err())
	node := root.LookupPath(cue.ParsePath("call0"))
	require.True(b, node.Exists())

	b.Run("marshal the call node", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := node.MarshalJSON(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("marshal only $params", func(b *testing.B) {
		b.ReportAllocs()
		params := node.LookupPath(cue.ParsePath("$params"))
		for i := 0; i < b.N; i++ {
			if _, err := params.MarshalJSON(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("fill the answer back", func(b *testing.B) {
		b.ReportAllocs()
		path := cue.ParsePath("call0")
		ret := map[string]any{"$returns": "k-0"}
		for i := 0; i < b.N; i++ {
			out := root.FillPath(path, ret)
			if out.Err() != nil {
				b.Fatal(out.Err())
			}
		}
	})
	b.Run("walk the whole value", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			count := 0
			root.Walk(func(cue.Value) bool { count++; return true }, nil)
		}
	})
}
