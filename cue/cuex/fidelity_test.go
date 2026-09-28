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
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"cuelang.org/go/cue"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// richReturns is what a provider that returns something other than a string
// hands back: nested structs, lists, numbers, nulls, and strings that mean
// something to CUE if they are not quoted properly.
type richReturns struct {
	Returns richValue `json:"$returns"`
}

type richValue struct {
	Str         string            `json:"str"`
	Empty       string            `json:"empty"`
	Zero        int               `json:"zero"`
	Negative    int               `json:"negative"`
	Float       float64           `json:"float"`
	True        bool              `json:"true"`
	False       bool              `json:"false"`
	EmptyList   []string          `json:"emptyList"`
	List        []int             `json:"list"`
	Nested      nestedValue       `json:"nested"`
	Map         map[string]string `json:"map"`
	ListOfMaps  []map[string]any  `json:"listOfMaps"`
	Interpolate string            `json:"interpolate"`
	Quoted      string            `json:"quoted"`
	Newlines    string            `json:"newlines"`
	Unicode     string            `json:"unicode"`
	Braces      string            `json:"braces"`
	Hash        string            `json:"hash"`
	Dollar      string            `json:"dollar"`
}

type nestedValue struct {
	Deep struct {
		Deeper []map[string]string `json:"deeper"`
	} `json:"deep"`
}

func richFor(key string) richValue {
	v := richValue{
		Str:         "value-" + key,
		Zero:        0,
		Negative:    -17,
		Float:       1.5,
		True:        true,
		EmptyList:   []string{},
		List:        []int{1, 2, 3},
		Map:         map[string]string{"a": "1", "b": "2"},
		ListOfMaps:  []map[string]any{{"k": "v", "n": 1}, {"k": "w", "n": 2}},
		Interpolate: `\(notAnInterpolation)`,
		Quoted:      `he said "hello" and 'bye'`,
		Newlines:    "one\ntwo\tthree",
		Unicode:     "café → 日本語 🎉",
		Braces:      "{not: a struct}",
		Hash:        "#do is not a definition here",
		Dollar:      "$params $returns",
	}
	v.Nested.Deep.Deeper = []map[string]string{{"x": key}, {"y": key}}
	return v
}

// richCompiler serves richReturns, optionally marked safe to run alongside
// itself so the same data goes through the concurrent path.
func richCompiler(tb testing.TB, concurrent bool) *cuex.Compiler {
	tb.Helper()
	var fn cuexruntime.ProviderFn = cuexruntime.GenericProviderFn[waitParams, richReturns](
		func(_ context.Context, p *waitParams) (*richReturns, error) {
			return &richReturns{Returns: richFor(p.Params)}, nil
		})
	if concurrent {
		fn = cuexruntime.Concurrent(fn)
	}
	pkg, err := cuexruntime.NewInternalPackage("rich", `
package rich

#Get: {
	#do:       "get"
	#provider: "rich"
	#config: maxPerRender: 64
	$params:   string
	$returns?: {...}
}`, map[string]cuexruntime.ProviderFn{"get": fn})
	require.NoError(tb, err)
	return cuex.NewCompilerWithInternalPackages(pkg)
}

func richFan(n int, attr string) string {
	var b strings.Builder
	b.WriteString("import \"vela/rich\"\nreads: {\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\t\"%d\": rich.#Get & {$params: \"k-%d\"}\n", i, i)
	}
	b.WriteString("}")
	b.WriteString(attr)
	b.WriteString("\n")
	// read some of it back out, so the data has to survive being referenced
	b.WriteString("firstDeep: reads[\"0\"].$returns.nested.deep.deeper\n")
	b.WriteString("firstList: reads[\"0\"].$returns.list\n")
	return b.String()
}

// TestRichDataMatchesReference pushes nested structs, lists, nulls, numbers
// and strings that mean something to CUE through the resolver, and requires
// the same bytes the resolver this replaced produced - running the calls one
// at a time and together.
func TestRichDataMatchesReference(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{1, 3, 12} {
		for _, attr := range []string{"", " @concurrency(8)"} {
			t.Run(fmt.Sprintf("n=%d attr=%q", n, attr), func(t *testing.T) {
				src := richFan(n, attr)
				c := richCompiler(t, false)
				wantOut, wantErr := render(referenceResolve(ctx, c, buildValue(t, c, src)))
				require.Empty(t, wantErr)

				gotOut, gotErr := render(richCompiler(t, true).CompileString(ctx, src))
				require.Equal(t, wantErr, gotErr)
				require.Equal(t, wantOut, gotOut)

				// and the awkward values really are in there
				for _, want := range []string{
					`"café → 日本語 🎉"`, `"{not: a struct}"`, `"#do is not a definition here"`,
					`"$params $returns"`, `"emptyList":[]`, `"zero":0`,
					`"negative":-17`, `"float":1.5`,
				} {
					require.Contains(t, gotOut, want)
				}
			})
		}
	}
}

// TestRichDataConcurrentEqualsSequential compares the two paths against each
// other at a size the old resolver cannot reach, since a Go value takes a
// different route into the value when a pass ran its calls together.
func TestRichDataConcurrentEqualsSequential(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{50, 500} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			sequential, err := richCompiler(t, false).CompileString(ctx, richFan(n, ""))
			require.NoError(t, err)
			concurrent, err := richCompiler(t, true).
				CompileString(ctx, richFan(n, " @concurrency(64)"))
			require.NoError(t, err)

			want, err := sequential.MarshalJSON()
			require.NoError(t, err)
			got, err := concurrent.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, string(want), string(got))
			require.Contains(t, string(got), fmt.Sprintf("value-k-%d", n-1),
				"the last call's data must be in there")
		})
	}
}

// TestNilInAResultIsTopBothWays pins a cue conversion a provider author can
// trip over: a Go nil in a returned value does not become null, it becomes
// top, and the value will not marshal afterwards.
//
// That is cue's behaviour rather than the resolver's, and this checks both
// resolvers still do it, so the rewrite is not blamed for it and a change in
// it is noticed. The values are compared rather than the marshal error, which
// names the field's full path here and only the field itself before.
func TestNilInAResultIsTopBothWays(t *testing.T) {
	type returns struct {
		Value struct {
			Present string  `json:"present"`
			Absent  *string `json:"absent"`
		} `json:"$returns"`
	}
	fn := cuexruntime.GenericProviderFn[waitParams, returns](
		func(_ context.Context, p *waitParams) (*returns, error) {
			var r returns
			r.Value.Present = p.Params
			return &r, nil
		})
	pkg, err := cuexruntime.NewInternalPackage("nilp", `
package nilp

#Get: {
	#do:       "get"
	#provider: "nilp"
	#config: maxPerRender: 64
	$params:   string
	$returns?: {...}
}`, map[string]cuexruntime.ProviderFn{"get": cuexruntime.Concurrent(fn)})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)
	ctx := context.Background()
	src := "import \"vela/nilp\"\nreads: {\n\t\"0\": nilp.#Get & {$params: \"a\"}\n\t\"1\": nilp.#Get & {$params: \"b\"}\n} @concurrency(2)\n"

	before, err := referenceResolve(ctx, c, buildValue(t, c, src))
	require.NoError(t, err)
	after, err := c.CompileString(ctx, src)
	require.NoError(t, err)

	for _, v := range []struct {
		name  string
		value cue.Value
	}{{"before", before}, {"after", after}} {
		present := v.value.LookupPath(cue.ParsePath(`reads."0".$returns.present`))
		got, err := present.String()
		require.NoError(t, err, v.name)
		require.Equal(t, "a", got, v.name)

		absent := v.value.LookupPath(cue.ParsePath(`reads."0".$returns.absent`))
		require.True(t, absent.Exists(), "%s: the field is there", v.name)
		require.Error(t, absent.Validate(cue.Concrete(true)),
			"%s: and it is top rather than null", v.name)

		_, mErr := v.value.MarshalJSON()
		require.ErrorContains(t, mErr, `cannot convert incomplete value "_"`, v.name)
	}
}

// TestImportedComprehensionRevealsACall is the template-side half of a hole
// the resolver had: a provider package's own CUE is built into the value, and
// a definition there can wrap a call in a comprehension. A template with no
// comprehension of its own still has to be searched again afterwards, or the
// call the package revealed never runs and nothing says so.
func TestImportedComprehensionRevealsACall(t *testing.T) {
	var calls int
	echo := cuexruntime.GenericProviderFn[waitParams, waitReturns](
		func(_ context.Context, p *waitParams) (*waitReturns, error) {
			calls++
			return &waitReturns{Returns: "ok-" + p.Params}, nil
		})
	pkg, err := cuexruntime.NewInternalPackage("chain", `
package chain

#Echo: {
	#do:       "echo"
	#provider: "chain"
	$params:   string
	$returns?: string
}

#Chain: {
	in:    string
	first: #Echo & {$params: in}
	if first.$returns != _|_ {
		second: #Echo & {$params: first.$returns}
		out:    second.$returns
	}
}`, map[string]cuexruntime.ProviderFn{"echo": echo})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)
	// the template has no comprehension; the package does
	src := "import \"vela/chain\"\nrun: chain.#Chain & {in: \"seed\"}\n"

	calls = 0
	out, err := c.CompileString(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, 2, calls, "the call the package revealed must run")
	got, err := out.LookupPath(cue.ParsePath("run.out")).String()
	require.NoError(t, err)
	require.Equal(t, "ok-ok-seed", got)
}

// TestNativeResultKeepsDefinitionsWhenBatched is the other half. A pass with
// one result writes it straight into the value; a pass with several renders
// them as syntax first, and rendering drops definitions unless asked not to.
// A definition is how one provider's result carries another call, so dropping
// it loses that call - and only shows up once two calls run together.
func TestNativeResultKeepsDefinitionsWhenBatched(t *testing.T) {
	var produced, consumed int
	producer := cuexruntime.NativeProviderFn(func(_ context.Context, v cue.Value) (cue.Value, error) {
		produced++
		return v.FillPath(cue.ParsePath("$returns"), v.Context().CompileString(`{
			nested: {#do: "count", #provider: "carry", $params: "from-native"}
		}`)), nil
	})
	counter := cuexruntime.GenericProviderFn[waitParams, waitReturns](
		func(_ context.Context, p *waitParams) (*waitReturns, error) {
			consumed++
			return &waitReturns{Returns: "ran-" + p.Params}, nil
		})
	pkg, err := cuexruntime.NewInternalPackage("carry", `
package carry

#Make: {
	#do:       "make"
	#provider: "carry"
	$params:   string
	$returns?: {...}
}`, map[string]cuexruntime.ProviderFn{"make": producer, "count": counter})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)
	ctx := context.Background()

	for name, src := range map[string]string{
		"one result in the pass":      "import \"vela/carry\"\na: carry.#Make & {$params: \"a\"}",
		"several results in the pass": "import \"vela/carry\"\na: carry.#Make & {$params: \"a\"}\nb: carry.#Make & {$params: \"b\"}\nc: carry.#Make & {$params: \"c\"}",
	} {
		t.Run(name, func(t *testing.T) {
			produced, consumed = 0, 0
			out, err := c.CompileString(ctx, src)
			require.NoError(t, err)
			require.Equal(t, produced, consumed,
				"every call a native provider wrote must run: produced %d, ran %d", produced, consumed)

			bs, err := out.MarshalJSON()
			require.NoError(t, err)
			require.Contains(t, string(bs), "ran-from-native")
		})
	}
}

// TestPartialResultsSurviveAnError: whatever a round managed before the error
// stays in the value it returns. A caller looking at the value to see how far
// it got - which is the whole point of a deadline - should see the calls that
// did run.
func TestPartialResultsSurviveAnError(t *testing.T) {
	fn := cuexruntime.GenericProviderFn[waitParams, waitReturns](
		func(_ context.Context, p *waitParams) (*waitReturns, error) {
			if p.Params == "boom" {
				return nil, fmt.Errorf("refused")
			}
			return &waitReturns{Returns: "ok-" + p.Params}, nil
		})
	pkg, err := cuexruntime.NewInternalPackage("partial", `
package partial

#Get: {
	#do:       "get"
	#provider: "partial"
	#config: maxPerRender: 64
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"get": fn})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)
	ctx := context.Background()
	// a, b succeed; c fails. All three are one level, run in walk order.
	src := `import "vela/partial"
a: partial.#Get & {$params: "a"}
b: partial.#Get & {$params: "b"}
c: partial.#Get & {$params: "boom"}`

	out, err := c.Resolve(ctx, buildValue(t, c, src))
	require.Error(t, err)
	for _, want := range []struct{ path, value string }{{"a", "ok-a"}, {"b", "ok-b"}} {
		got, gErr := out.LookupPath(cue.ParsePath(want.path + ".$returns")).String()
		require.NoError(t, gErr, "%s should have been kept", want.path)
		require.Equal(t, want.value, got)
	}
}

// TestInputTypeWithOtherFieldsStillSeesTheNode: reading only $params is an
// optimisation, and it has to be off for an input type that declares anything
// else, or a provider outside this repo silently stops getting it.
func TestInputTypeWithOtherFieldsStillSeesTheNode(t *testing.T) {
	type wideInput struct {
		Params string `json:"$params"`
		Do     string `json:"#do"`
		Other  string `json:"other"`
	}
	var seen wideInput
	fn := cuexruntime.GenericProviderFn[wideInput, waitReturns](
		func(_ context.Context, in *wideInput) (*waitReturns, error) {
			seen = *in
			return &waitReturns{Returns: in.Params}, nil
		})
	pkg, err := cuexruntime.NewInternalPackage("wide", `
package wide

#Get: {
	#do:       "get"
	#provider: "wide"
	$params:   string
	other?:    string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"get": fn})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)

	_, err = c.CompileString(context.Background(),
		"import \"vela/wide\"\nx: wide.#Get & {$params: \"p\", other: \"o\"}\n")
	require.NoError(t, err)
	require.Equal(t, "p", seen.Params)
	require.Equal(t, "o", seen.Other, "a sibling field must still be filled from the node")
}

// TestConcurrencyZeroIsSerial: a template writing @concurrency(0) is turning
// it off, and a number that cannot be read is not a request for anything.
// Neither may be taken as the default, which would turn concurrency on.
func TestConcurrencyZeroIsSerial(t *testing.T) {
	const n, latency = 6, 30 * time.Millisecond
	for name, attr := range map[string]string{
		"zero":       " @concurrency(0)",
		"negative":   " @concurrency(-4)",
		"unreadable": ` @concurrency("lots")`,
		"none named": " @concurrency()",
	} {
		t.Run(name, func(t *testing.T) {
			w := &waiter{latency: latency}
			_, err := waitingCompiler(t, w, true).
				CompileString(context.Background(), fanTemplate(n, attr))
			require.NoError(t, err)
			if name == "none named" {
				// the default is NumCPU, which is 1 on a one-CPU runner and
				// would make "ran them together" the wrong thing to expect
				want := goruntime.NumCPU()
				if want > n {
					want = n
				}
				require.EqualValues(t, want, w.peak.Load(),
					"@concurrency() asks for one per CPU")
				return
			}
			require.Equal(t, int32(1), w.peak.Load(), "%s must run calls one at a time", name)
		})
	}
}

// TestMutationCanRevealACall: a mutation replaces the value wholesale, so a
// comprehension it brings with it is not in the template and not in any
// package. The value has to be searched again afterwards or the call that
// comprehension reveals never runs, and nothing says so.
//
// WithIntraResolveMutation is how KubeVela injects config, so this is a path
// that carries real templates.
func TestMutationCanRevealACall(t *testing.T) {
	var calls int
	echo := cuexruntime.GenericProviderFn[waitParams, waitReturns](
		func(_ context.Context, p *waitParams) (*waitReturns, error) {
			calls++
			return &waitReturns{Returns: "ok-" + p.Params}, nil
		})
	pkg, err := cuexruntime.NewInternalPackage("probe", `
package probe

#Echo: {#do: "echo", #provider: "probe", $params: string, $returns?: string}`,
		map[string]cuexruntime.ProviderFn{"echo": echo})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)

	injected := `a: {#do: "echo", #provider: "probe", $params: "seed", $returns?: string}
if a.$returns != _|_ if a.$returns != "" {
	b: {#do: "echo", #provider: "probe", $params: a.$returns, $returns?: string}
}`
	out, err := c.CompileStringWithOptions(context.Background(), `z: 1`,
		cuex.WithIntraResolveMutation("inject", func(_ context.Context, v cue.Value) (cue.Value, error) {
			return v.Unify(v.Context().CompileString(injected)), nil
		}))
	require.NoError(t, err)
	require.Equal(t, 2, calls, "the call the mutation's comprehension revealed must run")
	got, err := out.LookupPath(cue.ParsePath("b.$returns")).String()
	require.NoError(t, err)
	require.Equal(t, "ok-ok-seed", got)
}

// TestConcurrentPartialResultsSurviveAnError: calls that were in flight
// together have already done whatever they do, so one of them failing must not
// throw away what the others returned. A caller reading the value to record
// what it changed would otherwise lose them.
func TestConcurrentPartialResultsSurviveAnError(t *testing.T) {
	fn := cuexruntime.Concurrent(cuexruntime.GenericProviderFn[waitParams, waitReturns](
		func(_ context.Context, p *waitParams) (*waitReturns, error) {
			time.Sleep(5 * time.Millisecond)
			if p.Params == "k-3" {
				return nil, fmt.Errorf("refused")
			}
			return &waitReturns{Returns: "ok-" + p.Params}, nil
		}))
	pkg, err := cuexruntime.NewInternalPackage("partpar", `
package partpar

#Get: {
	#do:       "get"
	#provider: "partpar"
	#config: maxPerRender: 64
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"get": fn})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)

	var b strings.Builder
	b.WriteString("import \"vela/partpar\"\nreads: {\n")
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&b, "\t\"%d\": partpar.#Get & {$params: \"k-%d\"}\n", i, i)
	}
	b.WriteString("} @concurrency(6)\n")

	out, err := c.Resolve(context.Background(), buildValue(t, c, b.String()))
	require.Error(t, err)
	kept := 0
	for i := 0; i < 6; i++ {
		got, gErr := out.LookupPath(cue.ParsePath(fmt.Sprintf(`reads."%d".$returns`, i))).String()
		if gErr == nil {
			require.Equal(t, fmt.Sprintf("ok-k-%d", i), got)
			kept++
		}
	}
	require.Equal(t, 5, kept, "every call that finished must have kept its result")
}

// TestComputedLabelRevealsACall covers a call that the build cannot produce an
// arc for. The label has to be computed from another call's output, so until
// that call has run there is no field to find, and a resolver that stops when
// the calls it first saw are done never finds it. The value looks finished and
// nothing reports otherwise, which is the part that makes it worth a test.
func TestComputedLabelRevealsACall(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	for name, src := range map[string]string{
		"interpolated label": `
import "vela/base64"
a: base64.#Encode & {$params: "x"}
b: {"\(a.$returns)": base64.#Encode & {$params: "y"}}`,
		"label in parens": `
import "vela/base64"
a: base64.#Encode & {$params: "x"}
b: {(a.$returns): base64.#Encode & {$params: "y"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := c.CompileString(context.Background(), src)
			require.NoError(t, err)
			bs, err := got.MarshalJSON()
			require.NoError(t, err)
			require.JSONEq(t,
				`{"a":{"$params":"x","$returns":"eA=="},"b":{"eA==":{"$params":"y","$returns":"eQ=="}}}`,
				string(bs), "the call under the computed label has to run too")
		})
	}
}
