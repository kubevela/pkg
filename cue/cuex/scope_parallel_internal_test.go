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
	"fmt"
	"strings"
	"sync"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"
)

// scopeOfParallel is scopeOf with each field's subtree read on its own
// goroutine. The syntax is plain Go structs and nothing writes to it, so
// unlike the value there is no question of whether this is allowed, only of
// whether it is worth it.
func scopeOfParallel(f *ast.File, imports []*build.Instance) callScope {
	providers := providerImports(f, imports)
	decls, ok := topLevel(f)
	if !ok {
		return nil
	}
	named := make([]bool, len(decls))
	var wg sync.WaitGroup
	for i := range decls {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			named[i] = holdsACall(decls[i].value, providers)
		}(i)
	}
	wg.Wait()
	return scopeFrom(decls, named)
}

func scopeSource(extra int) string {
	var b strings.Builder
	b.WriteString("import \"vela/ph\"\n")
	b.WriteString(phaseWorkload)
	b.WriteString("\ncall0: ph.#Do & {$params: \"k\"}\n")
	for i := 0; i < extra; i++ {
		fmt.Fprintf(&b, "\nextra%d: {a: \"x\", b: [1, 2, 3], c: {d: {e: %d, f: [{g: 1}, {h: 2}]}}}\n", i, i)
	}
	return b.String()
}

// TestScopeParallelAgrees: the two have to give the same answer, or the faster
// one is only faster at being wrong.
func TestScopeParallelAgrees(t *testing.T) {
	c := phaseCompiler(t)
	imports := c.PackageManager.GetImports()
	for _, extra := range []int{0, 5, 60} {
		f, err := parser.ParseFile("-", scopeSource(extra), parser.ParseComments)
		require.NoError(t, err)
		want := scopeOf(f, imports)
		// Two empty scopes agree about nothing. The source has to name a
		// package the compiler registered, or neither side finds a call and
		// the comparison holds whatever the parallel one does.
		require.NotEmpty(t, want, "the fixture has to give the scope something to find")
		require.Equal(t, want, scopeOfParallel(f, imports))
	}
}

// BenchmarkScopeSerialVsParallel: reading the syntax is already cheap against
// walking the value, so the question is whether splitting it pays for the
// goroutines at all.
func BenchmarkScopeSerialVsParallel(b *testing.B) {
	c := phaseCompiler(b)
	imports := c.PackageManager.GetImports()
	for _, extra := range []int{0, 20, 60, 200} {
		f, err := parser.ParseFile("-", scopeSource(extra), parser.ParseComments)
		require.NoError(b, err)
		b.Run(fmt.Sprintf("fields=%d/serial", extra+3), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = scopeOf(f, imports)
			}
		})
		b.Run(fmt.Sprintf("fields=%d/parallel", extra+3), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = scopeOfParallel(f, imports)
			}
		})
	}
}
