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
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/cue/util"
)

// referenceResolve is the resolver as it stood before batching: re-walk the
// whole value after every call, and fill each result at the root. It is kept
// verbatim as the oracle for TestResolveMatchesReference, so the optimised
// resolver has something to be equivalent *to* rather than only a set of
// hand-written expectations.
func referenceResolve(ctx context.Context, in *cuex.Compiler, value cue.Value) (cue.Value, error) {
	newValue := value
	executed := map[string]bool{}
	providers := in.PackageManager.GetProviders()
	for {
		if ddl, ok := ctx.Deadline(); ok && ddl.Before(time.Now()) {
			return newValue, cuex.ResolveTimeoutErr{}
		}
		var next *cue.Value
		util.Iterate(newValue, func(v cue.Value) (stop bool) {
			_, done := executed[v.Path().String()]
			fn, _ := v.LookupPath(cue.ParsePath("#do")).String()
			if !done && fn != "" {
				next = &v
				return true
			}
			return false
		})
		if next == nil {
			break
		}
		fn, _ := next.LookupPath(cue.ParsePath("#do")).String()
		prdName, _ := next.LookupPath(cue.ParsePath("#provider")).String()
		prd, found := providers[prdName]
		if !found {
			return newValue, cuex.ProviderNotFoundErr(prdName)
		}
		f := prd.GetProviderFn(fn)
		if f == nil {
			return newValue, cuex.ProviderFnNotFoundErr{Provider: prdName, Fn: fn}
		}
		val, err := f.Call(ctx, *next)
		if err != nil {
			return newValue, cuex.NewFunctionCallError(val, err)
		}
		newValue = newValue.FillPath(next.Path(), val)
		executed[next.Path().String()] = true
	}
	return newValue, nil
}

func buildValue(t testing.TB, c *cuex.Compiler, src string) cue.Value {
	t.Helper()
	bi := build.NewContext().NewInstance("", nil)
	bi.Imports = c.PackageManager.GetImports()
	f, err := parser.ParseFile("-", src, parser.ParseComments)
	require.NoError(t, err)
	require.NoError(t, bi.AddSyntax(f))
	return cuecontext.New().BuildInstance(bi)
}

// render is what a caller ultimately gets out of a resolved value: the
// marshalled result, or the error. Comparing this rather than the cue.Value
// is the point - it is the rendered output that has to stay correct.
func render(v cue.Value, err error) (string, string) {
	if err != nil {
		return "", err.Error()
	}
	bs, mErr := v.MarshalJSON()
	if mErr != nil {
		return "", "marshal: " + mErr.Error()
	}
	return string(bs), ""
}

// TestResolveMatchesReference is the regression net for the batching rewrite:
// for every corpus template, the optimised resolver must render byte for byte
// what the pre-batching one rendered, key order included, and fail with the
// same error where it failed.
func TestResolveMatchesReference(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	cases := map[string]resolveCase{}
	for name, tc := range resolveCorpus {
		cases[name] = tc
	}
	cases["manifest with 3 calls"] = resolveCase{Template: manifestTemplate(3, 3)}
	cases["manifest with 8 calls"] = resolveCase{Template: manifestTemplate(8, 6)}
	cases["40 independent calls"] = resolveCase{Template: widthTemplate(40)}
	cases["12 chained calls"] = resolveCase{Template: depthTemplate(12)}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.Divergent != "" {
				t.Skipf("deliberate divergence: %s", tc.Divergent)
			}
			wantOut, wantErr := render(referenceResolve(ctx, c, buildValue(t, c, tc.Template)))
			gotOut, gotErr := render(c.Resolve(ctx, buildValue(t, c, tc.Template)))
			require.Equal(t, wantErr, gotErr, "error differs from the reference resolver")
			require.Equal(t, wantOut, gotOut, "rendered output differs from the reference resolver")

			// and the corpus' own expectation, so a bug shared by both
			// resolvers still gets caught
			if tc.WantErr != "" {
				require.Contains(t, gotErr, tc.WantErr)
			} else {
				require.Empty(t, gotErr)
			}
		})
	}
}

// TestResolveDivergences pins the cases where batching is deliberately not
// equivalent, so the difference is a decision on record rather than a drift.
func TestResolveDivergences(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	t.Run("forward references now resolve instead of failing", func(t *testing.T) {
		src := resolveCorpus["forward reference, consumer declared first"].Template

		_, refErr := render(referenceResolve(ctx, c, buildValue(t, c, src)))
		require.Contains(t, refErr, "function call error for a",
			"the old resolver is expected to fail on a forward reference")

		out, err := render(c.Resolve(ctx, buildValue(t, c, src)))
		require.Empty(t, err, "the batching resolver runs b first and resolves a")
		require.JSONEq(t, `{"a":{"$params":"eA==","$returns":"ZUE9PQ=="},"b":{"$params":"x","$returns":"eA=="}}`, out)
	})

	t.Run("a forward reference across nesting resolves too", func(t *testing.T) {
		src := resolveCorpus["forward reference across nesting"].Template

		_, refErr := render(referenceResolve(ctx, c, buildValue(t, c, src)))
		require.Contains(t, refErr, "function call error for first.inner",
			"the old resolver is expected to fail on a forward reference")

		out, err := render(c.Resolve(ctx, buildValue(t, c, src)))
		require.Empty(t, err, "the batching resolver runs second.inner first")
		require.JSONEq(t,
			`{"first":{"inner":{"$params":"eA==","$returns":"ZUE9PQ=="}},`+
				`"second":{"inner":{"$params":"x","$returns":"eA=="}}}`, out)
	})

	t.Run("a template that already worked is unaffected", func(t *testing.T) {
		src := resolveCorpus["back reference, producer declared first"].Template
		want, wantErr := render(referenceResolve(ctx, c, buildValue(t, c, src)))
		require.Empty(t, wantErr)
		got, gotErr := render(c.Resolve(ctx, buildValue(t, c, src)))
		require.Empty(t, gotErr)
		require.Equal(t, want, got)
	})
}

// TestCallsRunBeforeAFailureIsReported is the second deliberate difference,
// and the one with teeth. A call whose parameters never settle is not ready,
// so it waits while the calls that are ready run, and only fails once nothing
// else is left. The resolver this replaced took calls in walk order and
// stopped at the first one that failed, so the calls after it never ran.
//
// Nothing renders differently. What changes is how much has happened by the
// time the error comes back, which for a provider that writes to a cluster is
// the difference between one resource applied and all of them.
//
// This is the cost of batching rather than an oversight: running the ready
// calls is the whole point, and which of them would have come after a failure
// in walk order is not knowable before they run.
func TestCallsRunBeforeAFailureIsReported(t *testing.T) {
	var ran []string
	fn := cuexruntime.GenericProviderFn[struct {
		Params string `json:"$params"`
	}, struct {
		Returns string `json:"$returns"`
	}](func(_ context.Context, p *struct {
		Params string `json:"$params"`
	}) (*struct {
		Returns string `json:"$returns"`
	}, error) {
		ran = append(ran, p.Params)
		return &struct {
			Returns string `json:"$returns"`
		}{Returns: "ok"}, nil
	})
	pkg, err := cuexruntime.NewInternalPackage("order", `
package order

#Get: {#do: "get", #provider: "order", $params: string, $returns?: string}`,
		map[string]cuexruntime.ProviderFn{"get": fn})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)

	// a can never resolve - its parameters are left as a type. This is the
	// shape a definition with an unresolved disjunction in its parameters has.
	src := `import "vela/order"
a: order.#Get & {$params: string}
b: order.#Get & {$params: "b"}
c: order.#Get & {$params: "c"}`

	ran = nil
	_, refErr := referenceResolve(context.Background(), c, buildValue(t, c, src))
	require.Error(t, refErr)
	require.Empty(t, ran, "the old resolver stopped at the first call and ran nothing")

	ran = nil
	_, newErr := c.Resolve(context.Background(), buildValue(t, c, src))
	require.Error(t, newErr)
	require.Equal(t, []string{"b", "c"}, ran,
		"the ready calls run before the one that cannot")
}

// TestResolveIsDeterministic guards the property the ast overlay exists to
// keep: repeated runs render identical bytes, so golden files and dry-run
// output do not move between releases.
func TestResolveIsDeterministic(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	for name, tc := range resolveCorpus {
		if tc.Divergent != "" || tc.WantErr != "" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			first, err := render(c.Resolve(ctx, buildValue(t, c, tc.Template)))
			require.Empty(t, err)
			for i := 0; i < 8; i++ {
				got, gotErr := render(c.Resolve(ctx, buildValue(t, c, tc.Template)))
				require.Empty(t, gotErr)
				require.Equal(t, first, got, "run %d rendered different bytes", i+2)
			}
		})
	}
}

// TestResolveCallsEachFunctionOnce guards against a batching bug that would be
// invisible in the rendered output: a pure function re-run is harmless, a
// kube.#Apply re-run is not.
func TestResolveCallsEachFunctionOnce(t *testing.T) {
	var calls int
	counting := cuexruntime.GenericProviderFn[struct {
		Params string `json:"$params"`
	}, struct {
		Returns string `json:"$returns"`
	}](func(_ context.Context, p *struct {
		Params string `json:"$params"`
	}) (*struct {
		Returns string `json:"$returns"`
	}, error) {
		calls++
		return &struct {
			Returns string `json:"$returns"`
		}{Returns: "ok-" + p.Params}, nil
	})

	pkg, err := cuexruntime.NewInternalPackage("count", `
package count

#Run: {
	#do:       "run"
	#provider: "count"
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"run": counting})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)

	for name, tc := range map[string]struct {
		src  string
		want int
	}{
		"three independent": {`
import "vela/count"
a: count.#Run & {$params: "a"}
b: count.#Run & {$params: "b"}
c: count.#Run & {$params: "c"}`, 3},
		"three chained": {`
import "vela/count"
a: count.#Run & {$params: "a"}
b: count.#Run & {$params: a.$returns}
c: count.#Run & {$params: b.$returns}`, 3},
		"one in a list": {`
import "vela/count"
xs: [count.#Run & {$params: "a"}, count.#Run & {$params: "b"}]`, 2},
		"revealed by a comprehension": {`
import "vela/count"
a: count.#Run & {$params: "a"}
if a.$returns != "" {
  b: count.#Run & {$params: "b"}
}`, 2},
	} {
		t.Run(name, func(t *testing.T) {
			calls = 0
			_, err := c.Resolve(context.Background(), buildValue(t, c, tc.src))
			require.NoError(t, err)
			require.Equal(t, tc.want, calls, "provider function call count")
		})
	}
}

// TestResolveRespectsDeadline keeps the timeout contract: the resolver stops
// between calls rather than running a whole batch past the deadline.
func TestResolveRespectsDeadline(t *testing.T) {
	slow := cuexruntime.GenericProviderFn[struct {
		Params string `json:"$params"`
	}, struct {
		Returns string `json:"$returns"`
	}](func(_ context.Context, p *struct {
		Params string `json:"$params"`
	}) (*struct {
		Returns string `json:"$returns"`
	}, error) {
		time.Sleep(80 * time.Millisecond)
		return &struct {
			Returns string `json:"$returns"`
		}{Returns: p.Params}, nil
	})
	pkg, err := cuexruntime.NewInternalPackage("slow", `
package slow

#Run: {
	#do:       "run"
	#provider: "slow"
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"run": slow})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)

	var b string
	for i := 0; i < 10; i++ {
		b += fmt.Sprintf("x%d: slow.#Run & {$params: \"v%d\"}\n", i, i)
	}
	src := "import \"vela/slow\"\n" + b

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = c.Resolve(ctx, buildValue(t, c, src))
	elapsed := time.Since(start)

	require.ErrorAs(t, err, &cuex.ResolveTimeoutErr{})
	require.Less(t, elapsed, 700*time.Millisecond,
		"resolver ran the whole batch past the deadline instead of stopping between calls")
}

// TestResolveWithNativeProvider covers the provider kind the resolver cannot
// make assumptions about: NativeProviderFn returns an arbitrary cue.Value, not
// just a $returns, so whatever it sets anywhere in the node has to survive.
func TestResolveWithNativeProvider(t *testing.T) {
	// deliberately writes outside $returns
	spread := cuexruntime.NativeProviderFn(func(_ context.Context, v cue.Value) (cue.Value, error) {
		in, err := v.LookupPath(cue.ParsePath("$params")).String()
		if err != nil {
			return v, err
		}
		v = v.FillPath(cue.ParsePath("$returns"), in+"-returns")
		v = v.FillPath(cue.ParsePath("sideband"), in+"-sideband")
		v = v.FillPath(cue.ParsePath("nested.deep.value"), in+"-nested")
		return v, nil
	})
	pkg, err := cuexruntime.NewInternalPackage("spread", `
package spread

#Run: {
	#do:       "run"
	#provider: "spread"
	$params:   string
	$returns?: string
	sideband?: string
	nested?: deep?: value?: string
}`, map[string]cuexruntime.ProviderFn{"run": spread})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)
	ctx := context.Background()

	for name, src := range map[string]string{
		"single": `
import "vela/spread"
a: spread.#Run & {$params: "a"}`,
		"two independent": `
import "vela/spread"
a: spread.#Run & {$params: "a"}
b: spread.#Run & {$params: "b"}`,
		"chained through a sideband field": `
import "vela/spread"
a: spread.#Run & {$params: "a"}
b: spread.#Run & {$params: a.sideband}`,
		"chained through a nested field": `
import "vela/spread"
a: spread.#Run & {$params: "a"}
b: spread.#Run & {$params: a.nested.deep.value}`,
	} {
		t.Run(name, func(t *testing.T) {
			wantOut, wantErr := render(referenceResolve(ctx, c, buildValue(t, c, src)))
			gotOut, gotErr := render(c.Resolve(ctx, buildValue(t, c, src)))
			require.Equal(t, wantErr, gotErr)
			require.Equal(t, wantOut, gotOut)
			require.Contains(t, gotOut, "-sideband", "fields written outside $returns were dropped")
			require.Contains(t, gotOut, "-nested", "fields written outside $returns were dropped")
		})
	}
}

// TestCompileMatchesReference covers the whole compile path rather than
// Resolve on its own: the skip for a template that can hold no call, and the
// stop for one that can reveal none. Neither is reached by calling Resolve
// directly, so without this the corpus would pass while never running them.
func TestCompileMatchesReference(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	cases := map[string]resolveCase{}
	for name, tc := range resolveCorpus {
		cases[name] = tc
	}
	cases["manifest with 1 call"] = resolveCase{Template: manifestTemplate(1, 3)}
	cases["manifest with 8 calls"] = resolveCase{Template: manifestTemplate(8, 6)}
	cases["manifest with no calls"] = resolveCase{Template: manifestTemplate(0, 3)}
	cases["fanout of 5"] = resolveCase{Template: fanoutTemplate(5)}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.Divergent != "" {
				t.Skipf("deliberate divergence: %s", tc.Divergent)
			}
			wantOut, wantErr := render(referenceResolve(ctx, c, buildValue(t, c, tc.Template)))
			gotOut, gotErr := render(c.CompileString(ctx, tc.Template))
			require.Equal(t, wantErr, gotErr, "error differs from the reference resolver")
			require.Equal(t, wantOut, gotOut, "rendered output differs from the reference resolver")
		})
	}
}

// TestNativeResultCanCarryACall pins why a native provider's result is treated
// as opaque. It builds its own cue.Value, and what is in it is its business -
// including, as here, another call. The resolver cannot stop after running the
// calls a build produced if one of them may have written another.
func TestNativeResultCanCarryACall(t *testing.T) {
	var produced, consumed int
	// hands back a node carrying a second call the template never wrote
	producer := cuexruntime.NativeProviderFn(func(_ context.Context, v cue.Value) (cue.Value, error) {
		produced++
		return v.FillPath(cue.ParsePath("$returns"), v.Context().CompileString(`{
			nested: {#do: "count", #provider: "hidden", $params: "from-native"}
		}`)), nil
	})
	counter := cuexruntime.GenericProviderFn[struct {
		Params string `json:"$params"`
	}, struct {
		Returns string `json:"$returns"`
	}](func(_ context.Context, p *struct {
		Params string `json:"$params"`
	}) (*struct {
		Returns string `json:"$returns"`
	}, error) {
		consumed++
		return &struct {
			Returns string `json:"$returns"`
		}{Returns: "ran-" + p.Params}, nil
	})

	pkg, err := cuexruntime.NewInternalPackage("hidden", `
package hidden

#Make: {
	#do:       "make"
	#provider: "hidden"
	$params:   string
	$returns?: {...}
}`, map[string]cuexruntime.ProviderFn{"make": producer, "count": counter})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)

	// no comprehension anywhere, so the only thing keeping the resolver
	// looking is that a native provider ran
	out, err := c.CompileString(context.Background(), `
import "vela/hidden"
seed: hidden.#Make & {$params: "seed"}`)
	require.NoError(t, err)

	require.Equal(t, 1, produced)
	require.Equal(t, 1, consumed, "the call the native provider wrote must still run")
	bs, err := out.MarshalJSON()
	require.NoError(t, err)
	require.Contains(t, string(bs), "ran-from-native")
}
