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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// A benchmark with one small provider package and a forty line value says
// little about a controller, which registers a couple of dozen packages and
// renders an application made of several components. These ask what each of
// those does to the cost of a call.

// manyPackages registers n copies of a small provider package, which is what a
// real compiler carries: the default set here plus what kubevela adds.
func manyPackages(tb testing.TB, n int) *cuex.Compiler {
	tb.Helper()
	fn := cuexruntime.GenericProviderFn[nothingParams, nothingReturns](
		func(_ context.Context, in *nothingParams) (*nothingReturns, error) {
			return &nothingReturns{Returns: in.Params}, nil
		})
	pkgs := make([]cuexruntime.Package, 0, n+1)
	real, err := cuexruntime.NewInternalPackage("nothing", `
package nothing

#Do: {
	#do:       "do"
	#provider: "nothing"
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"do": fn})
	require.NoError(tb, err)
	pkgs = append(pkgs, real)
	for i := 0; i < n; i++ {
		// each carries a definition block of the size a provider package has
		var body strings.Builder
		fmt.Fprintf(&body, "package filler%d\n", i)
		for d := 0; d < 6; d++ {
			fmt.Fprintf(&body, `
#Op%d: {
	#do:       "op%d"
	#provider: "filler%d"
	$params: {a?: string, b?: int, c?: [...string], d?: {...}}
	$returns?: {...}
}
`, d, d, i)
		}
		filler, err := cuexruntime.NewInternalPackage(
			fmt.Sprintf("filler%d", i), body.String(), map[string]cuexruntime.ProviderFn{})
		require.NoError(tb, err)
		pkgs = append(pkgs, filler)
	}
	return cuex.NewCompilerWithInternalPackages(pkgs...)
}

// BenchmarkPackageCount asks what registering more provider packages does to a
// compile that uses exactly one of them. Every compile attaches them all as
// imports, whether the template names them or not.
func BenchmarkPackageCount(b *testing.B) {
	src := withCalls(1)
	ctx := context.Background()
	for _, n := range []int{0, 5, 10, 20} {
		c := manyPackages(b, n)
		b.Run(fmt.Sprintf("packages=%d", n+1), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := c.CompileString(ctx, src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// biggerWorkload repeats the workload n times over, the way an application of
// several components is one value rather than several.
func biggerWorkload(components, calls int) string {
	var b strings.Builder
	b.WriteString("import \"vela/nothing\"\n")
	for i := 0; i < components; i++ {
		// each copy in its own struct, so its parameter and context are its
		// own. Renaming the labels alone left every reference to them, which
		// are written parameter.image and context.name, pointing at nothing.
		fmt.Fprintf(&b, "\nc%d: {%s}\n", i, realWorkload)
	}
	for i := 0; i < calls; i++ {
		fmt.Fprintf(&b, "\ncall%d: nothing.#Do & {$params: \"k-%d\"}\n", i, i)
	}
	return b.String()
}

// BenchmarkValueSize asks how the cost of one call moves as the value it sits
// in grows, which is the thing the profile said it follows.
func BenchmarkValueSize(b *testing.B) {
	c := nothingCompiler(b)
	ctx := context.Background()
	for _, components := range []int{1, 2, 4, 8} {
		src := biggerWorkload(components, 1)
		b.Run(fmt.Sprintf("components=%d", components), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := c.CompileString(ctx, src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
