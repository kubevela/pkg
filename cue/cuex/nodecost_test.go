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
	"encoding/base64"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
)

// What a resolved call leaves behind, and what it costs to leave it.
//
// A template reads a call's $returns. Everything else in the node is there
// because the definition was unified in to make the call: the #do and
// #provider that named it, the schema its parameters were checked against,
// and the slot its answer was declared in. None of that is read once the
// answer is in, but all of it stays in the value, once per call, for the
// rest of the render.

// TestWhatACallNodeHolds prints the node so a change to it is visible rather
// than inferred.
func TestWhatACallNodeHolds(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	v, err := c.CompileString(context.Background(), `
import "vela/base64"
x: base64.#Encode & {$params: "hello"}
out: x.$returns
`)
	require.NoError(t, err)

	node := v.LookupPath(cue.ParsePath("x"))
	bs, err := format.Node(node.Syntax(cue.All()))
	require.NoError(t, err)
	t.Logf("a resolved call node:\n%s", bs)

	var fields []string
	it, err := node.Fields(cue.All())
	require.NoError(t, err)
	for it.Next() {
		fields = append(fields, it.Selector().String())
	}
	t.Logf("fields on the node: %v", fields)

	// One of these is read and three are there because the definition was
	// unified in to make the call. That is the whole observation, so it is
	// worth failing on rather than printing.
	require.Equal(t, []string{"#do", "#provider", "$params", "$returns"}, fields,
		"a resolved call holds its answer and the three fields that named it")
}

// What a resolved call costs against what its answer alone costs: the same
// answers, at the same paths, with no call ever made.
//
// One test rather than two, because the gap is the finding and neither
// number says anything on its own. It is also why holding every answer at
// once is worth avoiding: most of what is held is not the answer.
func TestACallCostsMoreThanItsAnswer(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	perNode := func(src string, n int) int {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		v, err := c.CompileString(ctx, src)
		require.NoError(t, err)
		// Collected before reading, or an automatic collection landing
		// mid-compile decides the number: it frees what the compile threw
		// away and shrinks the heap, and it does that most to the template
		// that allocates most, which is the one being measured.
		runtime.GC()
		runtime.ReadMemStats(&after)
		out := int(after.HeapAlloc-before.HeapAlloc) / max(n, 1)
		runtime.KeepAlive(v)
		return out
	}

	answersOnly := func(n int) string {
		// a Builder, not += in a loop: at four thousand nodes that copies
		// the whole accumulated string every time and the test spends
		// longer building its input than measuring anything
		var b strings.Builder
		b.WriteString("out: \"x\"\n")
		for i := 0; i < n; i++ {
			// what the call answered, at the path the call sat at
			fmt.Fprintf(&b, "x%d: {$returns: %q}\n", i, base64Of(fmt.Sprintf("hello-%d", i)))
		}
		return b.String()
	}

	t.Logf("%6s %14s %14s %8s", "n", "per call", "per answer", "ratio")
	var wide [2]int
	for _, n := range []int{100, 1000, 4000} {
		call := perNode(widthTemplate(n), n)
		answer := perNode(answersOnly(n), n)
		t.Logf("%6d %12dB %12dB %7.2fx", n, call, answer, float64(call)/float64(answer))
		if n == 4000 {
			wide = [2]int{call, answer}
		}
	}

	// Measured at five point three times at four thousand; asserted at
	// twice, since both sides are heap readings and the claim is that most
	// of a resolved call is not its answer.
	require.Greater(t, wide[0], wide[1]*2,
		"a resolved call should cost multiples of the answer it holds")
}

// base64Of is what vela/base64 #Encode answers for a string.
func base64Of(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// TestWhereTheNodeCostIs splits the difference between a resolved call and a
// bare answer. A node carrying every field a call has, but written as plain
// data rather than unified with the definition, says whether the cost is the
// fields or the unification. That decides what a rewrite has to remove: if
// the fields are cheap, only the answer need survive and the call site has to
// be replaced outright; if the unification is what costs, the definition can
// go and the fields can stay.
func TestWhereTheNodeCostIs(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	shapes := []struct {
		name string
		node func(i int) string
	}{
		// nil means widthTemplate, the real thing
		{"resolved call, definition unified", nil},
		// The labels are quoted, so they are ordinary string fields and
		// not the definitions a call names itself by. Unquoted this shape
		// was a call the resolver would run, which is not what it is here
		// to weigh.
		{"every field, as plain data", func(i int) string {
			return fmt.Sprintf(`x%d: {"#do": "encode", "#provider": "base64", $params: %q, $returns: %q}`,
				i, fmt.Sprintf("hello-%d", i), base64Of(fmt.Sprintf("hello-%d", i)))
		}},
		{"params and answer, no #do", func(i int) string {
			return fmt.Sprintf(`x%d: {$params: %q, $returns: %q}`,
				i, fmt.Sprintf("hello-%d", i), base64Of(fmt.Sprintf("hello-%d", i)))
		}},
		{"the answer alone", func(i int) string {
			return fmt.Sprintf(`x%d: {$returns: %q}`, i, base64Of(fmt.Sprintf("hello-%d", i)))
		}},
	}

	const n = 4000
	cost := map[string]int{}
	for _, shape := range shapes {
		src := widthTemplate(n)
		if shape.node != nil {
			var b strings.Builder
			b.WriteString("out: \"x\"\n")
			for i := 0; i < n; i++ {
				b.WriteString(shape.node(i))
				b.WriteString("\n")
			}
			src = b.String()
		}
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		v, err := c.CompileString(ctx, src)
		require.NoError(t, err)
		// See above: read after a collection, so what is left is what the
		// value holds rather than whatever had not been swept yet.
		runtime.GC()
		runtime.ReadMemStats(&after)
		cost[shape.name] = int(after.HeapAlloc-before.HeapAlloc) / n
		t.Logf("SPLIT %-36s %8dKB live  %6dB per node",
			shape.name, after.HeapAlloc/1024, cost[shape.name])
		runtime.KeepAlive(v)
	}

	// The ordering is the answer to the question in the comment above, so
	// it is asserted rather than left to be read off the log. Both ends
	// cost: unifying the definition in costs over holding the same fields
	// as data, and holding the fields costs over holding the answer alone.
	// Measured at 8226, 3489, 2208 and 1552 bytes a node, read after a
	// collection so the numbers are what is held rather than what had not
	// been swept.
	require.Greater(t, cost["resolved call, definition unified"],
		cost["every field, as plain data"],
		"unifying the definition in costs more than the same fields as data")
	require.Greater(t, cost["every field, as plain data"],
		cost["params and answer, no #do"],
		"the fields a call names itself by cost something to hold")
	require.GreaterOrEqual(t, cost["params and answer, no #do"],
		cost["the answer alone"],
		"and the parameters cost something over the answer on its own")
}
