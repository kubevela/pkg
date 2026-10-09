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
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// genCompiler is a compiler whose calls record the order they run in: #Do
// answers with a Go value, #Native builds its own cue.Value as a legacy
// provider does, and #Stop refuses, ending the resolve the way a workflow
// step's wait does. #Guarded is a package definition with a guard of its own.
func genCompiler(t testing.TB) (*cuex.Compiler, func() []string) {
	var mu sync.Mutex
	var ran []string
	record := func(s string) {
		mu.Lock()
		ran = append(ran, s)
		mu.Unlock()
	}
	type in struct {
		Params string `json:"$params"`
	}
	type out struct {
		Returns string `json:"$returns"`
	}
	pkg, err := cuexruntime.NewInternalPackage("gen", `
package gen

#Do: {
	#do:       "do"
	#provider: "gen"
	$params:   string
	$returns?: string
}

#Native: {
	#do:       "native"
	#provider: "gen"
	$params:   string
	$returns?: string
}

#Stop: {
	#do:       "stop"
	#provider: "gen"
	$params:   string
}

#Wrap: {...}

#Guarded: {
	tag:   string
	probe: #Do & {$params: "probe-" + tag}
	if probe.$returns != "" {
		after: #Do & {$params: "after-" + tag}
	}
}
`, map[string]cuexruntime.ProviderFn{
		"do": cuexruntime.GenericProviderFn[in, out](func(_ context.Context, p *in) (*out, error) {
			record(p.Params)
			return &out{Returns: p.Params}, nil
		}),
		"native": legacyFn(func(value cue.Value) (cue.Value, error) {
			p, err := value.LookupPath(cue.ParsePath("$params")).String()
			if err != nil {
				return value, err
			}
			record(p)
			return value.FillPath(cue.ParsePath("$returns"), p), nil
		}),
		"stop": cuexruntime.GenericProviderFn[in, out](func(_ context.Context, p *in) (*out, error) {
			record(p.Params)
			return nil, fmt.Errorf("stopped at %s", p.Params)
		}),
	})
	require.NoError(t, err)
	return cuex.NewCompilerWithInternalPackages(pkg), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), ran...)
	}
}

// genTemplate writes a template whose every read is of a result written
// earlier in it, so the order the template writes its calls in is the order
// they must run in. A guard or a computed label sits under a named field, as a
// definition writes one: what a comprehension or a computed label adds to the
// top level comes after the declared fields in CUE's own order, which the
// template does not set, and which the resolver this replaced saw differently
// from one call to the next. Each statement is one of the shapes a calls' order can
// turn on: a guard, a loop, a computed label, a let, a named result, nesting,
// a package definition with a guard of its own, a call that ends the resolve.
func genTemplate(r *rand.Rand) string {
	var b strings.Builder
	b.WriteString("import \"vela/gen\"\n\n")
	var results []string // expressions reading an earlier call's result
	n := 3 + r.Intn(6)
	stopAt := -1
	if r.Intn(2) == 0 {
		stopAt = r.Intn(n)
	}
	def := func() string {
		if r.Intn(3) == 0 {
			return "gen.#Native"
		}
		return "gen.#Do"
	}
	param := func(k int) string {
		if len(results) > 0 && r.Intn(2) == 0 {
			return fmt.Sprintf(`"s%d-\(%s)"`, k, results[r.Intn(len(results))])
		}
		return fmt.Sprintf(`"s%d"`, k)
	}
	earlier := func() string {
		if len(results) == 0 {
			return `"seed"`
		}
		return results[r.Intn(len(results))]
	}
	for k := 0; k < n; k++ {
		if k == stopAt {
			fmt.Fprintf(&b, "stop%d: gen.#Stop & {$params: \"stop%d\"}\n", k, k)
			continue
		}
		switch r.Intn(14) {
		case 0, 1:
			fmt.Fprintf(&b, "c%d: %s & {$params: %s}\n", k, def(), param(k))
			results = append(results, fmt.Sprintf("c%d.$returns", k))
		case 2:
			if len(results) == 0 {
				fmt.Fprintf(&b, "c%d: gen.#Do & {$params: \"s%d\"}\n", k, k)
				results = append(results, fmt.Sprintf("c%d.$returns", k))
				continue
			}
			fmt.Fprintf(&b, "n%d: %s\n", k, earlier())
			results = append(results, fmt.Sprintf("n%d", k))
		case 3:
			fmt.Fprintf(&b, "let v%d = %s\n", k, earlier())
			results = append(results, fmt.Sprintf("v%d", k))
		case 4:
			fmt.Fprintf(&b, "g%d: {\n\tif %s != \"\" {\n\t\tcall: %s & {$params: %s}\n\t}\n}\n", k, earlier(), def(), param(k))
		case 5:
			fmt.Fprintf(&b, "w%d: {\n\tif %s != \"\" {\n\t\tfor i in [0, 1] {\n\t\t\t\"\\(i)\": %s & {$params: \"w%d-\\(i)\"}\n\t\t}\n\t}\n}\n",
				k, earlier(), def(), k)
		case 6:
			fmt.Fprintf(&b, "f%d: {\n\tfor i, x in [%s] {\n\t\t\"\\(i)\": %s & {$params: \"f%d-\\(x)\"}\n\t}\n}\n", k, earlier(), def(), k)
		case 7:
			fmt.Fprintf(&b, "k%d: \"k-\\(%s)\": %s & {$params: \"k%d\"}\n", k, earlier(), def(), k)
		case 8:
			fmt.Fprintf(&b, "d%d: a: b: %s & {$params: %s}\n", k, def(), param(k))
			results = append(results, fmt.Sprintf("d%d.a.b.$returns", k))
		case 9:
			fmt.Fprintf(&b, "l%d: [%s & {$params: \"l%d-0\"}, %s & {$params: \"l%d-1\"}]\n", k, def(), k, def(), k)
			results = append(results, fmt.Sprintf("l%d[1].$returns", k))
		case 10:
			fmt.Fprintf(&b, "p%d: gen.#Guarded & {tag: \"t%d\"}\n", k, k)
			results = append(results, fmt.Sprintf("p%d.probe.$returns", k))
		case 11:
			fmt.Fprintf(&b, "t%d: {\n\tif %s != \"\" && %s != \"\" {\n\t\tcall: %s & {$params: %s}\n\t}\n}\n",
				k, earlier(), earlier(), def(), param(k))
		case 12:
			fmt.Fprintf(&b, "x%d: y: z: {\n\tif %s != \"\" {\n\t\tinner: {\n\t\t\tif %s != \"\" {\n\t\t\t\tcall: %s & {$params: %s}\n\t\t\t}\n\t\t}\n\t}\n}\n",
				k, earlier(), earlier(), def(), param(k))
			results = append(results, fmt.Sprintf("x%d.y.z.inner.call.$returns", k))
		case 13:
			fmt.Fprintf(&b, "u%d: gen.#Wrap & {\n\tlet w = %s\n\tcall: %s & {$params: \"u%d-\\(w)\"}\n}\n", k, earlier(), def(), k)
			results = append(results, fmt.Sprintf("u%d.call.$returns", k))
		}
	}
	return b.String()
}

// TestResolveMatchesTheOneAtATimeOrder runs generated templates through the
// resolver and through the resolver it replaced, which ran the first call it
// found and then searched again, and requires the same calls in the same
// order and the same result. Every read in a generated template is of a call
// written before it, so that order is the template's own.
func TestResolveMatchesTheOneAtATimeOrder(t *testing.T) {
	ctx := context.Background()
	// CUEX_DIFFERENTIAL_CASES runs more, to hunt locally
	cases := int64(400)
	if n, err := strconv.ParseInt(os.Getenv("CUEX_DIFFERENTIAL_CASES"), 10, 64); err == nil && n > 0 {
		cases = n
	}
	ok := 0
	for seed := int64(0); seed < cases; seed++ {
		src := genTemplate(rand.New(rand.NewSource(seed)))

		refCompiler, refOrder := genCompiler(t)
		wantOut, wantErr := render(referenceResolve(ctx, refCompiler, buildValue(t, refCompiler, src)))

		c, order := genCompiler(t)
		gotOut, gotErr := render(c.CompileString(ctx, src))

		if !assertSameRun(t, seed, src, refOrder(), order(), wantErr, gotErr, wantOut, gotOut) {
			return
		}
		if wantErr == "" {
			ok++
		}
	}
	// a generator whose templates mostly fail to build tests nothing
	require.Greater(t, int64(ok), cases/3, "too few generated templates resolved cleanly")
}

func assertSameRun(t *testing.T, seed int64, src string, want, got []string, wantErr, gotErr, wantOut, gotOut string) bool {
	t.Helper()
	if !check(t, fmt.Sprint(want) == fmt.Sprint(got), "seed %d: calls ran in a different order\nwant %v\ngot  %v\n%s", seed, want, got, src) {
		return false
	}
	if !check(t, sameError(wantErr, gotErr), "seed %d: errors differ\nwant %q\ngot  %q\n%s", seed, wantErr, gotErr, src) {
		return false
	}
	return check(t, wantOut == gotOut, "seed %d: rendered differently\nwant %s\ngot  %s\n%s", seed, wantOut, gotOut, src)
}

// unreferencedLet is the one part of an error that is not stable: with more
// than one unreferenced let, CUE names whichever it meets first.
var unreferencedLet = regexp.MustCompile(`(unreferenced alias or let clause) \S+`)

func sameError(want, got string) bool {
	return unreferencedLet.ReplaceAllString(want, "$1") == unreferencedLet.ReplaceAllString(got, "$1")
}

func check(t *testing.T, ok bool, format string, args ...any) bool {
	t.Helper()
	if !ok {
		t.Errorf(format, args...)
	}
	return ok
}
