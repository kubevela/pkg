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

	gocontext "context"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
)

func bigSrc() string {
	src := realWorkload
	for i := 0; i < 60; i++ {
		src += fmt.Sprintf("\nextra%d: {a: \"x\", b: [1, 2, 3], c: {d: {e: %d}}}\n", i, i)
	}
	return src
}

// BenchmarkWalkSerialVsParallel: before asking whether splitting the walk is
// safe, ask whether it is faster. A walk is pointer chasing and small
// allocations, which shares badly across cores.
func BenchmarkWalkSerialVsParallel(b *testing.B) {
	src := bigSrc()
	cc := cuecontext.New()
	for name, walk := range map[string]func(cue.Value, *int64){
		"serial":   walkSerial,
		"parallel": walkParallel,
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				v := cc.CompileString(src).Unify(cc.CompileString(fmt.Sprintf("tag: %d", i)))
				b.StartTimer()
				var seen int64
				walk(v, &seen)
			}
		})
	}
}

// BenchmarkAstVsValueWalk is the other question: the AST is plain Go structs,
// already parsed, and needs no evaluation at all. What does walking it cost
// against walking the value?
func BenchmarkAstVsValueWalk(b *testing.B) {
	src := bigSrc()
	cc := cuecontext.New()

	b.Run("the value", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			v := cc.CompileString(src).Unify(cc.CompileString(fmt.Sprintf("tag: %d", i)))
			b.StartTimer()
			var seen int64
			walkSerial(v, &seen)
		}
	})

	f, err := parser.ParseFile("-", src, parser.ParseComments)
	require.NoError(b, err)
	b.Run("the syntax", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			seen := 0
			ast.Walk(f, func(ast.Node) bool { seen++; return true }, nil)
		}
	})
}

// TestHowManyNodesAreUnderACall asks what an AST pass could save: if the
// syntax says which top level fields can hold a call, the value walk could
// leave the rest alone. Most of a definition is the workload it renders, and
// no call is declared in there.
func TestHowManyNodesAreUnderACall(t *testing.T) {
	src := "import \"vela/nothing\"\n" + realWorkload + `
call0: nothing.#Do & {$params: "k-0"}
`
	c := nothingCompiler(t)
	v, err := c.CompileStringWithOptions(gocontext.Background(), src, cuex.DisableResolveProviderFunctions{})
	require.NoError(t, err)

	total, underCalls := 0, 0
	it, err := v.Fields(cue.Optional(true), cue.Hidden(true))
	require.NoError(t, err)
	for it.Next() {
		var n int64
		walkSerial(it.Value(), &n)
		total += int(n)
		if it.Selector().String() == "call0" {
			underCalls += int(n)
		}
	}
	t.Logf("nodes in the whole value: %d, under the field holding the call: %d (%.0f%%)",
		total, underCalls, 100*float64(underCalls)/float64(total))
}
