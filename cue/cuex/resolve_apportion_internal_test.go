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
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"
)

// Where a resolve's time goes, on the path a render actually takes, phase by
// phase. Allocation shares are a proxy; this is the thing itself.
//
// The phases do not overlap, which takes some care: applyResults renders each
// result as syntax itself, so timing that separately as well would count it
// twice and charge the second count to the unify.
func TestApportion(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()
	providers := c.PackageManager.GetProviders()
	ctx := context.Background()
	const runs = 300

	t.Log("calls   build    walk   params    call  putback  marshal | total")
	for _, n := range []int{1, 4, 8, 16} {
		src := apportionSrc(n)
		var build1, walk, params, called, putback, marshal time.Duration
		for i := 0; i < runs; i++ {
			s := time.Now()
			f, err := parser.ParseFile("-", src, parser.ParseComments)
			require.NoError(t, err)
			bi := build.NewContext().NewInstance("", nil)
			bi.Imports = imports
			require.NoError(t, bi.AddSyntax(f))
			value := cuecontext.New().BuildInstance(bi)
			build1 += time.Since(s)

			scope := scopeOf(f, imports)
			s = time.Now()
			pending := pendingCalls(value, map[string]bool{}, scope)
			walk += time.Since(s)
			require.Len(t, pending, n)

			var results []callResult
			for _, call := range pending {
				s = time.Now()
				fn, err := providerFn(providers, call)
				require.NoError(t, err)
				_ = call.value.LookupPath(paramsPath).Validate(cue.Concrete(true))
				params += time.Since(s)

				s = time.Now()
				ret, err := callProvider(ctx, fn, call)
				require.NoError(t, err)
				called += time.Since(s)

				results = append(results, callResult{call: call, ret: ret})
			}

			s = time.Now()
			out := applyResults(value, results)
			putback += time.Since(s)

			s = time.Now()
			_, err = out.MarshalJSON()
			require.NoError(t, err)
			marshal += time.Since(s)
		}
		us := func(d time.Duration) string {
			return fmt.Sprintf("%6.0f", float64(d.Nanoseconds())/float64(runs)/1000)
		}
		total := build1 + walk + params + called + putback + marshal
		t.Logf("%5d %s  %s   %s  %s   %s   %s | %s",
			n, us(build1), us(walk), us(params), us(called), us(putback), us(marshal), us(total))
	}
}

// What the put-back is made of: rendering each answer as syntax, building the
// collection of them into a value, and the one unify that writes them in.
func TestApportionThePutBack(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()
	providers := c.PackageManager.GetProviders()
	ctx := context.Background()
	const runs = 300

	t.Log("calls  syntax   build   unify | putback")
	for _, n := range []int{1, 4, 8, 16} {
		src := apportionSrc(n)
		var syntax, built, unify time.Duration
		for i := 0; i < runs; i++ {
			f, err := parser.ParseFile("-", src, parser.ParseComments)
			require.NoError(t, err)
			bi := build.NewContext().NewInstance("", nil)
			bi.Imports = imports
			require.NoError(t, bi.AddSyntax(f))
			value := cuecontext.New().BuildInstance(bi)

			pending := pendingCalls(value, map[string]bool{}, scopeOf(f, imports))
			require.Len(t, pending, n)
			var results []callResult
			for _, call := range pending {
				fn, err := providerFn(providers, call)
				require.NoError(t, err)
				ret, err := callProvider(ctx, fn, call)
				require.NoError(t, err)
				results = append(results, callResult{call: call, ret: ret})
			}

			s := time.Now()
			overlay := &overlayNode{}
			for _, result := range results {
				expr, ok := resultSyntax(value.Context(), result.ret, result.opaque)
				require.True(t, ok)
				require.True(t, overlay.set(result.call.fill, expr))
			}
			syntax += time.Since(s)

			s = time.Now()
			patch := value.Context().BuildExpr(overlay.expr())
			built += time.Since(s)

			s = time.Now()
			out := value.Unify(patch)
			require.NoError(t, out.Err())
			unify += time.Since(s)
		}
		us := func(d time.Duration) string {
			return fmt.Sprintf("%6.0f", float64(d.Nanoseconds())/float64(runs)/1000)
		}
		t.Logf("%5d %s  %s  %s | %s", n, us(syntax), us(built), us(unify), us(syntax+built+unify))
	}
}

// Every rearrangement of the resolve rests on the same claim: the build is
// lazy, the walk only forces the fields a call is under, and so there is an
// evaluation of the template to be saved by putting the answers back some
// other way.
//
// There is not. Finding the calls costs about what evaluating the whole
// template costs, so the walk is an evaluation in all but name, and a design
// that avoids the unify still pays for one: writing the answers into the
// syntax and building again, or reading the parameters out of a cut down
// file, both trade one evaluation for another and leave the walk where it was.
func BenchmarkFindingCallsAgainstEvaluatingEverything(b *testing.B) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()
	src := apportionSrc(8)

	built := func() (cue.Value, *ast.File) {
		f, err := parser.ParseFile("-", src, parser.ParseComments)
		require.NoError(b, err)
		bi := build.NewContext().NewInstance("", nil)
		bi.Imports = imports
		require.NoError(b, bi.AddSyntax(f))
		return cuecontext.New().BuildInstance(bi), f
	}

	b.Run("parse and build, touching nothing", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if v, _ := built(); v.Err() != nil {
				b.Fatal(v.Err())
			}
		}
	})
	b.Run("and then find the calls", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, f := built()
			if n := len(pendingCalls(v, map[string]bool{}, scopeOf(f, imports))); n != 8 {
				b.Fatalf("found %d", n)
			}
		}
	})
	b.Run("and then evaluate all of it instead", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, _ := built()
			if err := v.Validate(); err != nil {
				b.Fatal(err)
			}
			if _, err := v.MarshalJSON(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func apportionSrc(n int) string {
	var b strings.Builder
	b.WriteString("import \"vela/base64\"\n")
	b.WriteString(phaseWorkload)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\ncall%d: base64.#Encode & {$params: \"\\(context.name)-%d\"}\n", i, i)
	}
	return b.String()
}
