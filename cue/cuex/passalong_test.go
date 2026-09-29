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
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"github.com/stretchr/testify/require"
)

const chainPair = `
parameter: prefix: "p"
link0: {#do: "do", #provider: "x", $params: parameter.prefix}
link1: {#do: "do", #provider: "x", $params: link0.$returns}
`

// TestCanAResultReachTheNextCallWithoutTheRoot asks whether a chain has to pay
// for the whole value at every link.
//
// Only the next call's parameters need what the last one produced. If the
// result could be put where that call reads it, without folding it back into
// the value, a chain would cost one unify at the end instead of one per link.
func TestCanAResultReachTheNextCallWithoutTheRoot(t *testing.T) {
	cc := cuecontext.New()

	t.Run("unifying the answer into the node alone", func(t *testing.T) {
		root := cc.CompileString(chainPair)
		require.NoError(t, root.Err())
		node := root.LookupPath(cue.ParsePath("link1"))

		// what link1 waits for, offered to link1 rather than to the value
		answer := cc.CompileString(`{link0: {"$returns": "p"}}`)
		got := node.Unify(answer).LookupPath(cue.ParsePath("$params"))
		s, err := got.String()
		t.Logf("params after unifying into the node: %q err=%v", s, err)
		require.Error(t, err,
			"a reference is bound where it was written, so offering the answer "+
				"beside the node does not reach it")
	})

	t.Run("unifying the answer into the value", func(t *testing.T) {
		root := cc.CompileString(chainPair)
		require.NoError(t, root.Err())

		answer := cc.CompileString(`{link0: {"$returns": "p"}}`)
		got := root.Unify(answer).LookupPath(cue.ParsePath("link1.$params"))
		s, err := got.String()
		require.NoError(t, err, "which is why the value is what gets unified")
		require.Equal(t, "p", s)
	})

	t.Run("what the parameters look like as syntax", func(t *testing.T) {
		// The other way round would be to take what the call asks for as
		// syntax, put the answer where the reference is, and build that on
		// its own, with nothing left to be in scope for.
		//
		// The rewrite is not done here, and this does not assert that it
		// works. What it records is the syntax the rewrite would have to read,
		// which is the whole of why it was not built:
		// passalong_shape_test.go takes this shape as a definition writes it
		// and shows it arrives wrapped in export internals.
		root := cc.CompileString(chainPair)
		require.NoError(t, root.Err())
		params := root.LookupPath(cue.ParsePath("link1.$params"))

		bs, err := format.Node(params.Syntax(cue.Final()))
		require.NoError(t, err)
		// Logged before the assertion, because the log is what this subtest
		// is for: a CUE bump that changes the shape should print the shape it
		// changed to, not stop at a failed require and say nothing.
		t.Logf("link1 parameters as syntax: %s", bs)
		require.Contains(t, string(bs), "link0",
			"the reference is what a rewrite would have to find and replace")
	})
}

// BenchmarkReachTheNextCall puts a number on what a link costs each way: the
// whole value unified, against only what the next call reads.
func BenchmarkReachTheNextCall(b *testing.B) {
	cc := cuecontext.New()
	var big string
	for i := 0; i < 8; i++ {
		big += fmt.Sprintf("\nc%d: {%s}\n", i, realWorkload)
	}
	root := cc.CompileString(big + chainPair)
	require.NoError(b, root.Err())
	answer := cc.CompileString(`{link0: {"$returns": "p"}}`)

	b.Run("unify the whole value", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			got := root.Unify(answer).LookupPath(cue.ParsePath("link1.$params"))
			if _, err := got.String(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("build the parameters alone", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// what a rewrite would leave: the parameters with nothing to
			// resolve, built on their own
			got := cc.CompileString(`"p"`)
			if _, err := got.String(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
