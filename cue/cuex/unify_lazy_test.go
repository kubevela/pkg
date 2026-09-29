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

// Is what a level costs the unifying, or the reading afterwards?
//
// It matters for what could be done about it. If unifying is cheap and the
// cost is that a value built from another has nothing worked out yet, then
// every route that puts an answer into the value pays the same, whatever it
// does, and the only way round is not to put it there.
func BenchmarkUnifyThenRead(b *testing.B) {
	cc := cuecontext.New()
	var s strings.Builder
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&s, "\nc%d: {%s}\n", i, realWorkload)
	}
	s.WriteString("\nwaiting: {want: \"x\"}\n")
	root := cc.CompileString(s.String())
	require.NoError(b, root.Err())
	require.NoError(b, root.Validate()) // everything worked out once, here
	patch := cc.CompileString(`{answer: {"$returns": "a"}}`)
	// a patch that failed to compile is bottom, and unifying against bottom
	// would make every arm below look fast for the wrong reason
	require.NoError(b, patch.Err())

	b.Run("unify and read nothing", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = root.Unify(patch)
		}
	})
	b.Run("unify and read one small field", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out := root.Unify(patch)
			if _, err := out.LookupPath(cue.ParsePath("waiting.want")).String(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("read one small field, no unify", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := root.LookupPath(cue.ParsePath("waiting.want")).String(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
