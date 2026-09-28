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

	"github.com/kubevela/pkg/cue/cuex"
)

// The shapes over their useful range. Independent calls scale to thousands;
// chains do not, and the limit is CUE's rather than the resolver's - both this
// resolver and the one it replaced go from tens of milliseconds at depth 40 to
// hundreds at depth 50, so there is no point measuring past that.
var (
	scaleWidth    = []int{0, 1, 10, 100, 1000}
	scaleDepth    = []int{0, 1, 5, 10, 20, 30, 40}
	scaleManifest = []int{0, 1, 10, 100}
)

func BenchmarkScale(b *testing.B) {
	for _, shape := range []struct {
		name  string
		sizes []int
		src   func(int) string
	}{
		{"width", scaleWidth, widthTemplate},
		{"depth", scaleDepth, depthTemplate},
		{"manifest", scaleManifest, func(n int) string { return manifestTemplate(n, 6) }},
	} {
		for _, n := range shape.sizes {
			src := shape.src(n)
			b.Run(fmt.Sprintf("%s/%d/now", shape.name, n), func(b *testing.B) {
				benchScaleRun(b, src, false)
			})
			// the previous resolver is quadratic in the number of calls, so
			// past a few hundred it measures patience rather than anything else
			if n <= 100 {
				b.Run(fmt.Sprintf("%s/%d/reference", shape.name, n), func(b *testing.B) {
					benchScaleRun(b, src, true)
				})
			}
		}
	}
}

func benchScaleRun(b *testing.B, src string, reference bool) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		v := buildScaleValue(b, c, src)
		b.StartTimer()
		var err error
		if reference {
			_, err = referenceResolve(ctx, c, v)
		} else {
			_, err = c.Resolve(ctx, v)
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}

func buildScaleValue(b *testing.B, c *cuex.Compiler, src string) cue.Value {
	b.Helper()
	return buildValue(&testing.T{}, c, src)
}

// BenchmarkScaleCompile measures the whole compile rather than resolution
// alone. Two things only happen there: the skip for a template that can hold
// no call, and the stop for one that can reveal none.
func BenchmarkScaleCompile(b *testing.B) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	for _, shape := range []struct {
		name  string
		sizes []int
		src   func(int) string
	}{
		{"manifest", scaleManifest, func(n int) string { return manifestTemplate(n, 6) }},
		{"width", []int{0, 1, 10, 100}, widthTemplate},
		{"depth", []int{0, 1, 5, 10, 20}, depthTemplate},
	} {
		for _, n := range shape.sizes {
			src := shape.src(n)
			b.Run(fmt.Sprintf("%s/%d/now", shape.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := c.CompileString(ctx, src); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run(fmt.Sprintf("%s/%d/reference", shape.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					v := buildScaleValue(b, c, src)
					if _, err := referenceResolve(ctx, c, v); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
