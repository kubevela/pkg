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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// A pass with more than one call asks each provider for a cue.Value and then
// reads the syntax back out of it. Asking for the Go value instead and reading
// the syntax off that is the same answer by a shorter road, now that a Go
// result goes to syntax through JSON rather than through a value.
func BenchmarkTheTwoRoadsToAnAnswer(b *testing.B) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()
	providers := c.PackageManager.GetProviders()
	ctx := context.Background()

	f, err := parser.ParseFile("-", apportionSrc(8), parser.ParseComments)
	require.NoError(b, err)
	bi := build.NewContext().NewInstance("", nil)
	bi.Imports = imports
	require.NoError(b, bi.AddSyntax(f))
	value := cuecontext.New().BuildInstance(bi)

	pending := pendingCalls(value, map[string]bool{}, scopeOf(f, imports))
	require.Len(b, pending, 8)
	fn, err := providerFn(providers, pending[0])
	require.NoError(b, err)
	returning, ok := fn.(cuexruntime.ResultProviderFn)
	require.True(b, ok)

	b.Run("value out, syntax back, as a pass does now", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, call := range pending {
				ret, err := fn.Call(ctx, call.value)
				if err != nil {
					b.Fatal(err)
				}
				if _, ok := resultSyntax(value.Context(), ret, false); !ok {
					b.Fatal("no syntax")
				}
			}
		}
	})

	b.Run("go value out, syntax off that", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, call := range pending {
				ret, err := returning.CallForResult(ctx, call.value)
				if err != nil {
					b.Fatal(err)
				}
				if _, ok := resultSyntax(value.Context(), ret, false); !ok {
					b.Fatal("no syntax")
				}
			}
		}
	})
}

// The two roads have to arrive at the same answer.
func TestTheTwoRoadsAgree(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()
	providers := c.PackageManager.GetProviders()
	ctx := context.Background()

	f, err := parser.ParseFile("-", apportionSrc(3), parser.ParseComments)
	require.NoError(t, err)
	bi := build.NewContext().NewInstance("", nil)
	bi.Imports = imports
	require.NoError(t, bi.AddSyntax(f))
	cc := cuecontext.New()
	value := cc.BuildInstance(bi)

	pending := pendingCalls(value, map[string]bool{}, scopeOf(f, imports))
	require.Len(t, pending, 3)
	fn, err := providerFn(providers, pending[0])
	require.NoError(t, err)
	returning, ok := fn.(cuexruntime.ResultProviderFn)
	require.True(t, ok, "a provider that stopped offering the result form should fail the test, not panic it")

	for _, call := range pending {
		asValue, err := fn.Call(ctx, call.value)
		require.NoError(t, err)
		viaValue, ok := resultSyntax(cc, asValue, false)
		require.True(t, ok)

		asGo, err := returning.CallForResult(ctx, call.value)
		require.NoError(t, err)
		viaGo, ok := resultSyntax(cc, asGo, false)
		require.True(t, ok)

		want, err := cc.BuildExpr(viaValue).LookupPath(cue.ParsePath("$returns")).String()
		require.NoError(t, err)
		got, err := cc.BuildExpr(viaGo).LookupPath(cue.ParsePath("$returns")).String()
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}
