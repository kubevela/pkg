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
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// Holding back results nothing reads, and unifying them with a later level's,
// would make the intermediate overlays smaller. Whether that is worth doing
// turns on what a unify costs: if it follows the value it is applied to, the
// saving is nil, because every level pays for the whole value either way.
func BenchmarkUnifyCost(b *testing.B) {
	cc := cuecontext.New()

	base := func(components int) string {
		var s strings.Builder
		for i := 0; i < components; i++ {
			fmt.Fprintf(&s, "\nc%d: {%s}\n", i, realWorkload)
		}
		return s.String()
	}
	overlay := func(fields int) string {
		var s strings.Builder
		s.WriteString("{")
		for i := 0; i < fields; i++ {
			fmt.Fprintf(&s, "r%d: {\"$returns\": \"v-%d\"}, ", i, i)
		}
		s.WriteString("}")
		return s.String()
	}

	for _, components := range []int{1, 8} {
		root := cc.CompileString(base(components))
		require.NoError(b, root.Err())
		require.NoError(b, root.Validate())
		for _, fields := range []int{1, 8, 32} {
			patch := cc.CompileString(overlay(fields))
			require.NoError(b, patch.Err())
			b.Run(fmt.Sprintf("components=%d/results=%d", components, fields), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					out := root.Unify(patch)
					// a unify is lazy, so it has to be looked at to have
					// happened at all
					if out.Err() != nil {
						b.Fatal(out.Err())
					}
				}
			})
		}
	}
}

// BenchmarkUnifyVersusFill compares one unify of many results against one fill
// each, which is the other way a level could put its results back.
func BenchmarkUnifyVersusFill(b *testing.B) {
	cc := cuecontext.New()
	var s strings.Builder
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&s, "\nc%d: {%s}\n", i, realWorkload)
	}
	root := cc.CompileString(s.String())
	require.NoError(b, root.Err())

	var o strings.Builder
	o.WriteString("{")
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&o, "r%d: {\"$returns\": \"v-%d\"}, ", i, i)
	}
	o.WriteString("}")
	patch := cc.CompileString(o.String())
	require.NoError(b, patch.Err())

	b.Run("one unify of sixteen", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out := root.Unify(patch)
			if out.Err() != nil {
				b.Fatal(out.Err())
			}
		}
	})
	b.Run("sixteen fills", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out := root
			for j := 0; j < 16; j++ {
				out = out.FillPath(cue.ParsePath(fmt.Sprintf("r%d", j)),
					map[string]any{"$returns": fmt.Sprintf("v-%d", j)})
			}
			if out.Err() != nil {
				b.Fatal(out.Err())
			}
		}
	})
}
