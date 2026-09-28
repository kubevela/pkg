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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"github.com/stretchr/testify/require"
)

// The three below decide what a call is waiting for, which is what orders the
// levels. They are reached through the corpus, but only at the shapes the
// corpus happens to hold, so they are also worth asking directly: a wrong
// answer here runs a call before its input is ready, or holds one that was.

// TestIsUnder covers the string test for "this reference lands inside that
// call". A prefix alone is not enough, since reads and readsMore share one.
func TestIsUnder(t *testing.T) {
	for name, tt := range map[string]struct {
		ref, owner string
		want       bool
	}{
		"the call itself":          {"reads", "reads", true},
		"a field of it":            {"reads.$returns", "reads", true},
		"deeper inside it":         {"reads.a.b.c", "reads", true},
		"an index into it":         {`reads[0]`, "reads", true},
		"a field under an index":   {`reads[0].$returns`, "reads", true},
		"a longer sibling name":    {"readsMore", "reads", false},
		"a sibling under the name": {"readsMore.$returns", "reads", false},
		"somewhere else entirely":  {"writes.$returns", "reads", false},
		"the owner is longer":      {"reads", "reads.$returns", false},
		"a quoted step":            {`reads."0".$returns`, "reads", true},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, isUnder(tt.ref, tt.owner))
		})
	}
}

// TestOwningExecutedCall: a reference is settled once it lands inside a call
// that has already run, whatever depth it points at.
func TestOwningExecutedCall(t *testing.T) {
	executed := map[string]bool{"reads": true, `creates."1"`: true}
	for name, tt := range map[string]struct {
		ref  string
		want bool
	}{
		"straight at a call that ran":  {"reads", true},
		"inside a call that ran":       {"reads.$returns", true},
		"inside a quoted one that ran": {`creates."1".$returns`, true},
		"a sibling that did not run":   {`creates."2".$returns`, false},
		"a name sharing a prefix":      {"readsMore", false},
		"nothing to do with either":    {"parameter.image", false},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, owningExecutedCall(tt.ref, executed))
		})
	}
	require.False(t, owningExecutedCall("reads", map[string]bool{}),
		"nothing has run, so nothing is settled")
}

// TestCollectReferences covers what a call is found to be waiting on. It stops
// at the reference rather than following it, which is what keeps a deep chain
// from being walked end to end for every call in it.
func TestCollectReferences(t *testing.T) {
	cc := cuecontext.New()
	for name, tt := range map[string]struct {
		src  string
		want []string
	}{
		"no reference at all":     {`{a: 1, b: "x"}`, nil},
		"a plain reference":       {`{src: {v: 1}, out: src.v}`, []string{"src.v"}},
		"one inside a string":     {`{src: {v: "x"}, out: "\(src.v)"}`, []string{"src.v"}},
		"two in one expression":   {`{a: {v: 1}, b: {v: 2}, out: a.v + b.v}`, []string{"a.v", "b.v"}},
		"one inside a list":       {`{src: {v: 1}, out: [src.v, 2]}`, []string{"src.v"}},
		"one nested in a struct":  {`{src: {v: 1}, out: {deep: {here: src.v}}}`, []string{"src.v"}},
		"one behind an index":     {`{src: [{v: 1}], out: src[0].v}`, []string{"src[0].v"}},
		"the same one used twice": {`{src: {v: 1}, out: {a: src.v, b: src.v}}`, []string{"src.v", "src.v"}},
	} {
		t.Run(name, func(t *testing.T) {
			v := cc.CompileString(tt.src)
			require.NoError(t, v.Err())
			var got []reference
			require.True(t, collectReferences(v.LookupPath(cue.ParsePath("out")), &got, 0))
			keys := make([]string, 0, len(got))
			for _, r := range got {
				keys = append(keys, r.key)
			}
			if tt.want == nil {
				require.Empty(t, keys)
				return
			}
			require.ElementsMatch(t, tt.want, keys)
		})
	}
}

// TestCollectReferencesStopsAtTheDepthBound: a value deep enough to be worth
// giving up on is given up on, and saying so is what makes the caller treat
// the call as not ready rather than as waiting for nothing.
func TestCollectReferencesStopsAtTheDepthBound(t *testing.T) {
	var b strings.Builder
	b.WriteString("out: ")
	for i := 0; i < maxReferenceDepth+10; i++ {
		fmt.Fprintf(&b, "a%d: ", i)
	}
	b.WriteString("1")
	v := cuecontext.New().CompileString(b.String())
	require.NoError(t, v.Err())

	var got []reference
	require.False(t, collectReferences(v.LookupPath(cue.ParsePath("out")), &got, 0),
		"past the bound it has to report that it did not finish looking")

	// the same shape inside the bound, so the case above is about depth and
	// not about the shape being unreadable
	shallow := cuecontext.New().CompileString("out: a0: a1: a2: 1")
	require.NoError(t, shallow.Err())
	got = nil
	require.True(t, collectReferences(shallow.LookupPath(cue.ParsePath("out")), &got, 0))
}

// TestResultSyntax covers turning a result into syntax for the overlay. A
// native provider's result can carry another call in a definition, so those
// are kept; a Go value has none, and asking for them would drag each call's
// own #do and #provider into the collection.
func TestResultSyntax(t *testing.T) {
	cc := cuecontext.New()

	t.Run("a Go value", func(t *testing.T) {
		expr, ok := resultSyntax(cc, map[string]any{"$returns": "x"}, false)
		require.True(t, ok)
		require.Contains(t, exprString(t, cc, expr), `"x"`)
	})

	t.Run("a cue.Value needs no context", func(t *testing.T) {
		expr, ok := resultSyntax(nil, cc.CompileString(`{$returns: "x"}`), false)
		require.True(t, ok)
		require.Contains(t, exprString(t, cc, expr), `"x"`)
	})

	t.Run("a Go value with no context cannot be built", func(t *testing.T) {
		_, ok := resultSyntax(nil, map[string]any{"$returns": "x"}, false)
		require.False(t, ok, "there is nothing to compile it with")
	})

	t.Run("definitions are dropped unless asked for", func(t *testing.T) {
		val := cc.CompileString(`{$returns: "x", #keep: {a: 1}}`)
		plain, ok := resultSyntax(nil, val, false)
		require.True(t, ok)
		require.NotContains(t, exprString(t, cc, plain), "#keep")

		opaque, ok := resultSyntax(nil, val, true)
		require.True(t, ok)
		require.Contains(t, exprString(t, cc, opaque), "#keep",
			"a native result can carry another call in a definition")
	})

	t.Run("a value that cannot be built", func(t *testing.T) {
		_, ok := resultSyntax(cc, func() {}, false)
		require.False(t, ok, "a func is not something CUE can hold")
	})
}

// exprString renders syntax back to CUE text, which is the only way to see
// what the overlay would carry.
func exprString(t *testing.T, _ *cue.Context, expr ast.Expr) string {
	t.Helper()
	bs, err := format.Node(expr)
	require.NoError(t, err)
	return string(bs)
}

// TestOverlaySetRejects covers which paths the ordered overlay can hold. What
// it turns down matters more than what it takes: a rejected path falls back to
// filling the value directly, which is always correct but costs a unification,
// while wrongly accepting one writes the result somewhere it does not belong.
func TestOverlaySetRejects(t *testing.T) {
	expr := ast.NewString("x")
	for name, tt := range map[string]struct {
		path string
		want bool
	}{
		"a plain field":       {"out", true},
		"a nested field":      {"out.deep.here", true},
		"a quoted field":      {`out."0".$returns`, true},
		"a hidden field":      {"_hidden", false},
		"a definition":        {"#def", false},
		"a hidden field deep": {"out._hidden", false},
		"a definition deep":   {"out.#def", false},
		"a list index":        {"out[0]", false},
		"nothing at all":      {"", false},
	} {
		t.Run(name, func(t *testing.T) {
			root := &overlayNode{}
			require.Equal(t, tt.want, root.set(cue.ParsePath(tt.path), expr))
		})
	}
}

// TestOverlaySetWillNotClaimTwice: two results cannot write the same field, and
// neither can one write inside what another already holds. Either would lose a
// result, so both fall back to being filled one at a time.
func TestOverlaySetWillNotClaimTwice(t *testing.T) {
	expr := ast.NewString("x")

	root := &overlayNode{}
	require.True(t, root.set(cue.ParsePath("out.a"), expr))
	require.False(t, root.set(cue.ParsePath("out.a"), expr), "the same field twice")

	root = &overlayNode{}
	require.True(t, root.set(cue.ParsePath("out"), expr))
	require.False(t, root.set(cue.ParsePath("out.deeper"), expr), "inside a result already placed")

	root = &overlayNode{}
	require.True(t, root.set(cue.ParsePath("out.a"), expr))
	require.False(t, root.set(cue.ParsePath("out"), expr), "over a struct already holding one")

	root = &overlayNode{}
	require.True(t, root.set(cue.ParsePath("out.a"), expr))
	require.True(t, root.set(cue.ParsePath("out.b"), expr), "siblings are fine")
}
