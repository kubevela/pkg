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

	"cuelang.org/go/cue/cuecontext"
)

// A call is a struct unified with a definition, and CUE charges for that
// whether or not a resolver ever looks at it. Comparing a render against a
// template with no calls in it therefore counts that cost as the resolver's.
//
// answeredSrc is the same template with the answers already written in, so
// nothing is left to resolve. The gap between it and a real render is what the
// resolver itself costs.
func answeredSrc(n int) string {
	var b strings.Builder
	b.WriteString(realWorkload)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\ncall%d: {#do: \"do\", #provider: \"nothing\", $params: \"k-%d\", $returns: \"k-%d\"}\n", i, i, i)
	}
	return b.String()
}

func BenchmarkResolverOverhead(b *testing.B) {
	c := nothingCompiler(b)
	ctx := context.Background()
	cc := cuecontext.New()
	for _, n := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("calls=%d/answered", n), func(b *testing.B) {
			b.ReportAllocs()
			src := answeredSrc(n)
			for i := 0; i < b.N; i++ {
				if _, err := cc.CompileString(src).MarshalJSON(); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("calls=%d/resolved", n), func(b *testing.B) {
			b.ReportAllocs()
			src := withCalls(n)
			for i := 0; i < b.N; i++ {
				v, err := c.CompileString(ctx, src)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := v.MarshalJSON(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
