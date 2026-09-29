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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/util"
)

// BenchmarkWalkCost asks what a walk actually pays for.
//
// Iterate reaches a field with cue.Value.Fields, and that finalises the vertex
// it is reading, so the first walk of a value evaluates all of it. A second
// walk of the same value finds the work already done. The gap between them is
// evaluation, and it is what a resolve pays again every time it puts a result
// back and has to look at the value afresh.
func BenchmarkWalkCost(b *testing.B) {
	cc := cuecontext.New()

	b.Run("a value never walked before", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			// a new value each time, so nothing is evaluated yet
			v := cc.CompileString(realWorkload)
			require.NoError(b, v.Err())
			b.StartTimer()
			util.Iterate(v, func(cue.Value) bool { return false })
		}
	})

	b.Run("a value already walked", func(b *testing.B) {
		v := cc.CompileString(realWorkload)
		require.NoError(b, v.Err())
		util.Iterate(v, func(cue.Value) bool { return false })
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			util.Iterate(v, func(cue.Value) bool { return false })
		}
	})

	b.Run("a value just unified", func(b *testing.B) {
		v := cc.CompileString(realWorkload)
		require.NoError(b, v.Err())
		util.Iterate(v, func(cue.Value) bool { return false })
		patch := cc.CompileString(`extra: added: true`)
		require.NoError(b, patch.Err())
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			// what applyResults hands back: a value built from one that was
			// evaluated, which has to be evaluated again
			next := v.Unify(patch)
			b.StartTimer()
			util.Iterate(next, func(cue.Value) bool { return false })
		}
	})
}
