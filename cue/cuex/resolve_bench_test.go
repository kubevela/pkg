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

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
)

// benchShapes are the template shapes the resolver's cost depends on: how many
// calls there are, and whether they can run together.
var benchShapes = []struct {
	name string
	src  func() string
}{
	{"width/10", func() string { return widthTemplate(10) }},
	{"width/20", func() string { return widthTemplate(20) }},
	{"width/30", func() string { return widthTemplate(30) }},
	{"width/40", func() string { return widthTemplate(40) }},
	{"width/50", func() string { return widthTemplate(50) }},
	{"width/100", func() string { return widthTemplate(100) }},
	{"depth/2", func() string { return depthTemplate(2) }},
	{"depth/3", func() string { return depthTemplate(3) }},
	{"depth/5", func() string { return depthTemplate(5) }},
	{"depth/10", func() string { return depthTemplate(10) }},
	{"depth/20", func() string { return depthTemplate(20) }},
	{"fanout/5", func() string { return fanoutTemplate(5) }},
	{"fanout/20", func() string { return fanoutTemplate(20) }},
	{"mixed/4indep+2chain", func() string { return mixedTemplate(4, 2) }},
	{"mixed/4indep+5chain", func() string { return mixedTemplate(4, 5) }},
	{"manifest/3calls", func() string { return manifestTemplate(3, 3) }},
	{"manifest/8calls", func() string { return manifestTemplate(8, 6) }},
}

func BenchmarkResolve(b *testing.B) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	for _, shape := range benchShapes {
		src := shape.src()
		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				v := buildBenchValue(b, c, src)
				b.StartTimer()
				if _, err := c.Resolve(ctx, v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkResolveReference is the pre-batching resolver on the same shapes,
// so the two can be compared directly with benchstat.
func BenchmarkResolveReference(b *testing.B) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	for _, shape := range benchShapes {
		src := shape.src()
		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				v := buildBenchValue(b, c, src)
				b.StartTimer()
				if _, err := referenceResolve(ctx, c, v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func buildBenchValue(b *testing.B, c *cuex.Compiler, src string) cue.Value {
	b.Helper()
	return buildValue(b, c, src)
}

// TestResolveProducesEveryCall checks that each shape comes back with a
// $returns for every call it holds, whatever order the batching ran them in.
// It says nothing about how many passes that took, which the benchmarks
// measure and which no assertion here could tell apart.
func TestResolveProducesEveryCall(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		src   string
		calls int
	}{
		{"10 independent calls", widthTemplate(10), 10},
		{"10 chained calls", depthTemplate(10), 10},
		{"manifest with 8 calls", manifestTemplate(8, 6), 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := c.Resolve(ctx, buildValue(t, c, tc.src))
			require.NoError(t, err)
			bs, err := v.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, tc.calls, countResolved(string(bs)),
				"every call should have produced a $returns")
		})
	}
}

func countResolved(s string) int {
	n := 0
	for i := 0; i+len(`"$returns"`) <= len(s); i++ {
		if s[i:i+len(`"$returns"`)] == `"$returns"` {
			n++
		}
	}
	return n
}

var _ = fmt.Sprintf
