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
	"math"
	"sync/atomic"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// The corners nothing else reaches.
//
// The bugs so far were all shapes no test held: a field written twice, a
// selector onto a definition, an index into one, an optional hidden
// field, a whole float in company. Each was found by someone going
// looking rather than by the corpus. This is the going looking: batch
// arithmetic, empty loops, cancellation, values JSON cannot carry,
// results that are CUE rather than Go, and the prepass meeting the other
// things a compile can be asked to do.

var edgeRuns atomic.Int64

type edgeIn struct {
	Params string `json:"$params"`
}
type edgeOut struct {
	Returns string `json:"$returns"`
}

type wideIn struct {
	Params string `json:"$params"`
}
type wideOut struct {
	Returns struct {
		Zero     int               `json:"zero"`
		Neg      int               `json:"neg"`
		Big      int64             `json:"big"`
		Tiny     float64           `json:"tiny"`
		EmptyMap map[string]string `json:"emptyMap"`
		NilMap   map[string]string `json:"nilMap"`
		EmptyArr []string          `json:"emptyArr"`
		NilArr   []string          `json:"nilArr"`
		Uni      string            `json:"uni"`
		Braces   string            `json:"braces"`
		Nested   struct {
			Deep struct {
				V string `json:"v"`
			} `json:"deep"`
		} `json:"nested"`
	} `json:"$returns"`
}

type badIn struct {
	Params string `json:"$params"`
}

func edgePackage() cuexruntime.Package {
	pkg, err := cuexruntime.NewInternalPackage("edge", `
package edge

#Do: {
	#do:       "do"
	#provider: "edge"
	$params:   string
	$returns?: string
}

#Wide: {
	#do:       "wide"
	#provider: "edge"
	$params:   string
	$returns?: {...}
}

#Native: {
	#do:       "native"
	#provider: "edge"
	$params:   string
	$returns?: {said: string}
}

#Slow: {
	#do:       "slow"
	#provider: "edge"
	$params:   string
	$returns?: string
}
`, map[string]cuexruntime.ProviderFn{
		"do": cuexruntime.GenericProviderFn[edgeIn, edgeOut](
			func(_ context.Context, in *edgeIn) (*edgeOut, error) {
				edgeRuns.Add(1)
				return &edgeOut{Returns: in.Params}, nil
			}),
		"wide": cuexruntime.GenericProviderFn[wideIn, wideOut](
			func(_ context.Context, in *wideIn) (*wideOut, error) {
				out := &wideOut{}
				out.Returns.Zero = 0
				out.Returns.Neg = -7
				out.Returns.Big = math.MaxInt64
				out.Returns.Tiny = 0.000001
				out.Returns.EmptyMap = map[string]string{}
				out.Returns.EmptyArr = []string{}
				out.Returns.Uni = "héllo ✓ 世界"
				out.Returns.Braces = `{"a": 1} \(not interpolated)`
				out.Returns.Nested.Deep.V = "down"
				return out, nil
			}),
		// a provider that builds its own value, which may carry a
		// definition and so another call
		"native": cuexruntime.NativeProviderFn(
			func(_ context.Context, v cue.Value) (cue.Value, error) {
				p, _ := v.LookupPath(cue.ParsePath("$params")).String()
				return v.FillPath(cue.ParsePath("$returns"),
					v.Context().CompileString(fmt.Sprintf("{said: %q}", p))), nil
			}),
		"slow": cuexruntime.GenericProviderFn[edgeIn, edgeOut](
			func(ctx context.Context, in *edgeIn) (*edgeOut, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(2 * time.Millisecond):
				}
				edgeRuns.Add(1)
				return &edgeOut{Returns: in.Params}, nil
			}),
	})
	if err != nil {
		panic(err)
	}
	return pkg
}

func edgeLoop(n int, fn string) string {
	return fmt.Sprintf(`
import "vela/edge"
import "list"
_idx: list.Range(0, %d, 1)
_c: {for i in _idx {"\(i)": edge.%s & {$params: "p\(i)"}}}
out: {for i in _idx {"\(i)": _c["\(i)"].$returns}}
`, n, fn)
}

// Batch arithmetic. A run that divides the loop exactly, one that leaves
// a remainder of one, and one larger than the loop: all of them have to
// answer every iteration once and no more.
func TestEdgeBatchBoundaries(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(edgePackage())
	for _, tc := range []struct{ n, batch int }{
		{10, 10}, {10, 5}, {11, 5}, {10, 1}, {10, 100}, {1, 100}, {101, 10}, {100, 3},
	} {
		edgeRuns.Store(0)
		cuex.OptimiseStats.Calls.Store(0)
		v, err := c.CompileStringWithOptions(context.Background(), edgeLoop(tc.n, "#Do"),
			cuex.WithOptimise(cuex.OptimisePolicy{Enabled: true, Threshold: 1, Batch: tc.batch}))
		require.NoError(t, err, "n=%d batch=%d", tc.n, tc.batch)

		it, err := v.LookupPath(cue.ParsePath("out")).Fields()
		require.NoError(t, err)
		seen := 0
		for it.Next() {
			got, err := it.Value().String()
			require.NoError(t, err)
			require.Equal(t, "p"+it.Selector().Unquoted(), got)
			seen++
		}
		require.Equal(t, tc.n, seen, "n=%d batch=%d: every iteration should be there", tc.n, tc.batch)
		require.EqualValues(t, tc.n, edgeRuns.Load(),
			"n=%d batch=%d: every call once and no more", tc.n, tc.batch)
		// The count above is the same whether the batches answered these
		// calls or the prepass declined and the resolver did, so on its
		// own it says nothing about the batch arithmetic it is here for.
		require.EqualValues(t, tc.n, cuex.OptimiseStats.Calls.Load(),
			"n=%d batch=%d: the batches have to be what answered them", tc.n, tc.batch)
	}
}

// A loop over nothing.
func TestEdgeEmptyLoop(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(edgePackage())
	edgeRuns.Store(0)
	v, err := c.CompileStringWithOptions(context.Background(), `
import "vela/edge"
import "list"
_idx: list.Range(0, 0, 1)
_c: {for i in _idx {"\(i)": edge.#Do & {$params: "p"}}}
out: {for i in _idx {"\(i)": _c["\(i)"].$returns}}
`, cuex.WithOptimise(cuex.OptimisePolicy{Enabled: true, Threshold: 1}))
	require.NoError(t, err)
	bs, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
	require.NoError(t, err)
	require.JSONEq(t, "{}", string(bs))
	require.EqualValues(t, 0, edgeRuns.Load())
}

// Everything a Go result can hold that a conversion could mangle, with a
// second call beside it so both roads are taken.
func TestEdgeAwkwardResults(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(edgePackage())
	ctx := context.Background()

	alone, err := c.CompileString(ctx, `
import "vela/edge"
a: edge.#Wide & {$params: "x"}`)
	require.NoError(t, err)
	together, err := c.CompileString(ctx, `
import "vela/edge"
a: edge.#Wide & {$params: "x"}
b: edge.#Wide & {$params: "y"}`)
	require.NoError(t, err)

	for _, path := range []string{
		"a.$returns.zero", "a.$returns.neg", "a.$returns.big", "a.$returns.tiny",
		"a.$returns.emptyMap", "a.$returns.nilMap", "a.$returns.emptyArr",
		"a.$returns.nilArr", "a.$returns.uni", "a.$returns.braces",
		"a.$returns.nested.deep.v",
	} {
		one, oneErr := alone.LookupPath(cue.ParsePath(path)).MarshalJSON()
		two, twoErr := together.LookupPath(cue.ParsePath(path)).MarshalJSON()
		require.NoError(t, oneErr, path)
		require.NoError(t, twoErr, path)
		require.JSONEq(t, string(one), string(two),
			"%s should not depend on how many calls shared the pass", path)
	}
	big, err := alone.LookupPath(cue.ParsePath("a.$returns.big")).Int64()
	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), big, "a large int should survive intact")
}

// A provider that builds its own value: what is in it is that function's
// business and the resolver has to keep all of it.
func TestEdgeNativeResultThroughALoop(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(edgePackage())
	for _, policy := range []cuex.OptimisePolicy{
		{},
		{Enabled: true, Threshold: 1},
	} {
		t.Logf("EDGE native, prepass enabled=%v", policy.Enabled)
		v, err := c.CompileStringWithOptions(context.Background(), `
import "vela/edge"
import "list"
_idx: list.Range(0, 20, 1)
_c: {for i in _idx {"\(i)": edge.#Native & {$params: "p\(i)"}}}
out: {for i in _idx {"\(i)": _c["\(i)"].$returns.said}}
`, cuex.WithOptimise(policy))
		require.NoError(t, err, "enabled=%v", policy.Enabled)
		bs, err := v.LookupPath(cue.ParsePath(`out["7"]`)).MarshalJSON()
		require.NoError(t, err, "enabled=%v", policy.Enabled)
		require.JSONEq(t, `"p7"`, string(bs))
	}
}

// A context already done before the render starts.
func TestEdgeCancelledContext(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(edgePackage())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.CompileStringWithOptions(ctx, edgeLoop(200, "#Slow"),
		cuex.WithOptimise(cuex.OptimisePolicy{Enabled: true, Threshold: 1}))
	require.Error(t, err, "a cancelled context should stop the render")
}

// A deadline that expires partway through.
func TestEdgeDeadlineDuringALoop(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(edgePackage())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err := c.CompileStringWithOptions(ctx, edgeLoop(500, "#Slow"),
		cuex.WithOptimise(cuex.OptimisePolicy{Enabled: true, Threshold: 1}))
	require.Error(t, err, "a deadline that passes should stop the render")
}

// The prepass beside the other things a compile can be asked to do.
func TestEdgeWithTheOtherOptions(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(edgePackage())
	ctx := context.Background()
	on := cuex.WithOptimise(cuex.OptimisePolicy{Enabled: true, Threshold: 1})

	t.Run("resolution turned off answers nothing", func(t *testing.T) {
		edgeRuns.Store(0)
		_, err := c.CompileStringWithOptions(ctx, edgeLoop(50, "#Do"),
			cuex.DisableResolveProviderFunctions{}, on)
		require.NoError(t, err)
		require.EqualValues(t, 0, edgeRuns.Load(),
			"nothing should run when the caller said not to resolve")
	})

	t.Run("data filled in as a value", func(t *testing.T) {
		// A value from somewhere else cannot be filled into this one, so
		// the check is that the prepass changes nothing about that: the
		// two policies have to behave the same, whatever that is.
		cc := cuecontext.New()
		outcome := func(policy cuex.OptimisePolicy) string {
			var out string
			func() {
				defer func() {
					if r := recover(); r != nil {
						out = fmt.Sprintf("panic: %v", r)
					}
				}()
				_, err := c.CompileStringWithOptions(ctx, `
import "vela/edge"
import "list"
_idx: list.Range(0, 50, 1)
given: _
_c: {for i in _idx {"\(i)": edge.#Do & {$params: "\(given.tag)-\(i)"}}}
out: _c["3"].$returns
`, cuex.WithData("given", cc.CompileString(`{tag: "v1"}`)), cuex.WithOptimise(policy))
				out = fmt.Sprintf("err: %v", err)
			}()
			return out
		}
		off := outcome(cuex.OptimisePolicy{})
		with := outcome(cuex.OptimisePolicy{Enabled: true, Threshold: 1})
		t.Logf("EDGE filling a value from another context: off=%s", off)
		require.Equal(t, off, with,
			"whatever filling a foreign value does, the prepass must not change it")
	})

	t.Run("data filled in as a value from this compile", func(t *testing.T) {
		_, err := c.CompileStringWithOptions(ctx, `
import "vela/edge"
import "list"
_idx: list.Range(0, 50, 1)
given: _
_c: {for i in _idx {"\(i)": edge.#Do & {$params: "\(given.tag)-\(i)"}}}
out: _c["3"].$returns
`, cuex.WithData("given", map[string]any{"tag": "v1"}), on)
		require.NoError(t, err)
	})

	t.Run("a mutation between the build and the resolve", func(t *testing.T) {
		_, err := c.CompileStringWithOptions(ctx, edgeLoop(50, "#Do"), on,
			cuex.WithIntraResolveMutation("noop",
				func(_ context.Context, v cue.Value) (cue.Value, error) { return v, nil }))
		require.NoError(t, err)
	})

	t.Run("data filled in as syntax", func(t *testing.T) {
		expr, err := parser.ParseExpr("-", `{tag: "v2"}`)
		require.NoError(t, err)
		v, err := c.CompileStringWithOptions(ctx, `
import "vela/edge"
import "list"
_idx: list.Range(0, 50, 1)
given: _
_c: {for i in _idx {"\(i)": edge.#Do & {$params: "\(given.tag)-\(i)"}}}
out: _c["3"].$returns
`, cuex.WithData("given", expr), on)
		require.NoError(t, err)
		got, err := v.LookupPath(cue.ParsePath("out")).String()
		require.NoError(t, err)
		require.Equal(t, "v2-3", got)
	})
}

// Loops in places a loop can be that are not a plain top-level field.
func TestEdgeLoopsInOddPlaces(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(edgePackage())
	ctx := context.Background()
	on := cuex.WithOptimise(cuex.OptimisePolicy{Enabled: true, Threshold: 1})

	for _, tc := range []struct {
		name, src, path, want, wantErr string
	}{
		{
			name: "a loop under an optional field",
			src: `
import "vela/edge"
import "list"
_idx: list.Range(0, 30, 1)
c?: {for i in _idx {"\(i)": edge.#Do & {$params: "p\(i)"}}}
out: "done"`,
			path: "out", want: `"done"`,
		},
		{
			name: "a loop nested under another field",
			src: `
import "vela/edge"
import "list"
_idx: list.Range(0, 30, 1)
holder: inner: {for i in _idx {"\(i)": edge.#Do & {$params: "p\(i)"}}}
out: holder.inner["4"].$returns`,
			path: "out", want: `"p4"`,
		},
		{
			name: "a loop read whole by another field",
			src: `
import "vela/edge"
import "list"
_idx: list.Range(0, 30, 1)
_c: {for i in _idx {"\(i)": edge.#Do & {$params: "p\(i)"}}}
all: _c
out: all["4"].$returns`,
			path: "out", want: `"p4"`,
		},
		{
			name: "a loop whose parameters read context",
			src: `
import "vela/edge"
import "list"
context: {name: "app"}
_idx: list.Range(0, 30, 1)
_c: {for i in _idx {"\(i)": edge.#Do & {$params: "\(context.name)-\(i)"}}}
out: _c["4"].$returns`,
			path: "out", want: `"app-4"`,
		},
		{
			name: "two loops reading the same answers",
			src: `
import "vela/edge"
import "list"
_idx: list.Range(0, 30, 1)
_c: {for i in _idx {"\(i)": edge.#Do & {$params: "p\(i)"}}}
x: {for i in _idx {"\(i)": _c["\(i)"].$returns}}
y: {for i in _idx {"\(i)": _c["\(i)"].$returns}}
out: "\(x["4"])\(y["4"])"`,
			path: "out", want: `"p4p4"`,
		},
		{
			// A template reading past the end of what its own loop made.
			// The wantErr arm exists for this: a template the resolver
			// rejects must not come out rendered because the prepass
			// answered the loop first, and until there was a case using
			// it that branch ran for nothing.
			name: "a read past the end of the loop",
			src: `
import "vela/edge"
import "list"
_idx: list.Range(0, 30, 1)
_c: {for i in _idx {"\(i)": edge.#Do & {$params: "p\(i)"}}}
out: _c["99"].$returns`,
			path: "out", wantErr: "undefined field",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := func(opt cuex.CompileOption) (string, error) {
				v, err := c.CompileStringWithOptions(ctx, tc.src, opt)
				if err != nil {
					return "", err
				}
				bs, err := v.LookupPath(cue.ParsePath(tc.path)).MarshalJSON()
				return string(bs), err
			}
			offJSON, offErr := read(cuex.WithOptimise(cuex.OptimisePolicy{}))
			onJSON, onErr := read(on)

			if tc.wantErr != "" {
				// Both arms have to fail, and for the same reason: a
				// template that the resolver rejects must not render
				// because the prepass answered it first.
				require.Error(t, offErr, "the resolver")
				require.Contains(t, offErr.Error(), tc.wantErr, "the resolver")
				require.Error(t, onErr, "the prepass")
				require.Contains(t, onErr.Error(), tc.wantErr, "the prepass")
				return
			}
			require.NoError(t, offErr, "the resolver")
			require.NoError(t, onErr, "the prepass")
			require.JSONEq(t, tc.want, offJSON, "the resolver")
			require.JSONEq(t, offJSON, onJSON,
				"the prepass must render what the resolver renders")
		})
	}
}

// Keys that are not tidy identifiers.
func TestEdgeAwkwardKeys(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(edgePackage())
	ctx := context.Background()
	src := `
import "vela/edge"
_names: ["a-b", "c.d", "e f", "héllo", "0", "$x"]
_c: {for n in _names {"\(n)": edge.#Do & {$params: n}}}
out: {for n in _names {"\(n)": _c["\(n)"].$returns}}
`
	off, err := c.CompileStringWithOptions(ctx, src, cuex.WithOptimise(cuex.OptimisePolicy{}))
	require.NoError(t, err)
	offJSON, err := off.LookupPath(cue.ParsePath("out")).MarshalJSON()
	require.NoError(t, err)

	onV, err := c.CompileStringWithOptions(ctx, src,
		cuex.WithOptimise(cuex.OptimisePolicy{Enabled: true, Threshold: 1}))
	require.NoError(t, err)
	onJSON, err := onV.LookupPath(cue.ParsePath("out")).MarshalJSON()
	require.NoError(t, err)

	require.JSONEq(t, string(offJSON), string(onJSON),
		"a key the loop made should come back the same whoever answered it")
	require.Contains(t, string(onJSON), "héllo")
}
