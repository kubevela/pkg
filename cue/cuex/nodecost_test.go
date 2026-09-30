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
}

// TestCallNodeCost is the number the work is against: what a value holding n
// resolved calls costs, and how much of that is the answers.
func TestCallNodeCost(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	t.Logf("%6s %12s %14s", "calls", "live heap", "per call")
	for _, n := range []int{100, 1000, 4000} {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)

		v, err := c.CompileString(ctx, widthTemplate(n))
		require.NoError(t, err)

		runtime.ReadMemStats(&after)
		heap := after.HeapAlloc
		t.Logf("%6d %10dKB %12dB", n, heap/1024, int(heap-before.HeapAlloc)/max(n, 1))
		runtime.KeepAlive(v)
	}
}

// TestAnswersAloneCost is the floor: the same answers at the same paths, with
// no call ever made, so nothing of the definition is in the value. The gap
// between this and TestCallNodeCost is what returns-only is trying to close.
func TestAnswersAloneCost(t *testing.T) {
	cc := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	t.Logf("%6s %12s %14s", "nodes", "live heap", "per node")
	for _, n := range []int{100, 1000, 4000} {
		src := "out: \"x\"\n"
		for i := 0; i < n; i++ {
			// what the call answered, at the path the call sat at
			src += fmt.Sprintf("x%d: {$returns: %q}\n", i, base64Of(fmt.Sprintf("hello-%d", i)))
		}
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)

		v, err := cc.CompileString(ctx, src)
		require.NoError(t, err)

		runtime.ReadMemStats(&after)
		heap := after.HeapAlloc
		t.Logf("%6d %10dKB %12dB", n, heap/1024, int(heap-before.HeapAlloc)/max(n, 1))
		runtime.KeepAlive(v)
	}
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
		{"every field, as plain data", func(i int) string {
			return fmt.Sprintf(`x%d: {#do: "encode", #provider: "base64", $params: %q, $returns: %q}`,
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
	for _, shape := range shapes {
		src := "out: \"x\"\n"
		if shape.node == nil {
			src = widthTemplate(n)
		} else {
			for i := 0; i < n; i++ {
				src += shape.node(i) + "\n"
			}
		}
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		v, err := c.CompileString(ctx, src)
		require.NoError(t, err)
		runtime.ReadMemStats(&after)
		t.Logf("SPLIT %-36s %8dKB live  %6dB per node",
			shape.name, after.HeapAlloc/1024, int(after.HeapAlloc-before.HeapAlloc)/n)
		runtime.KeepAlive(v)
	}
}
