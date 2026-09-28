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
	"strings"
	"testing"
)

// A call that waits on another cannot go in the same level, so a chain costs
// one round each. A round is a walk of the whole value and a unify of the
// whole value, and the value is bigger each time, which is where a cost
// measured in microseconds per call turns into something else entirely.

func chainOn(components, depth int) string {
	var b strings.Builder
	b.WriteString("import \"vela/nothing\"\n")
	for i := 0; i < components; i++ {
		fmt.Fprintf(&b, "\nc%d: {%s}\n", i, realWorkload)
	}
	b.WriteString("\nlink0: nothing.#Do & {$params: \"start\"}\n")
	for i := 1; i < depth; i++ {
		fmt.Fprintf(&b, "link%d: nothing.#Do & {$params: link%d.$returns}\n", i, i-1)
	}
	return b.String()
}

// BenchmarkChainDepth: every link is a round of its own, on a value that keeps
// growing, so the cost per link is not the cost of the first one.
func BenchmarkChainDepth(b *testing.B) {
	c := nothingCompiler(b)
	ctx := context.Background()
	for _, depth := range []int{1, 2, 4, 8, 16} {
		src := chainOn(4, depth)
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := c.CompileString(ctx, src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkChainOnABigValue is the same chain against values of two sizes, to
// separate what a round costs from how many rounds there are.
func BenchmarkChainOnABigValue(b *testing.B) {
	c := nothingCompiler(b)
	ctx := context.Background()
	for _, components := range []int{1, 8} {
		for _, depth := range []int{1, 8} {
			src := chainOn(components, depth)
			b.Run(fmt.Sprintf("components=%d/depth=%d", components, depth), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := c.CompileString(ctx, src); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
