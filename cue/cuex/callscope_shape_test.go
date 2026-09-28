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

package cuex

import (
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"
)

// The scope exists to keep the walk off the manifest, so what it leaves out
// matters as much as what it takes.
//
// Every template that calls a provider also reads what the call returned, and
// the manifest reads the field holding that. If reading a call's output
// counted as holding a call, the scope would take the field, then the manifest
// reading it, and there would be no scope left.
func TestTheScopeLeavesTheManifestOut(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()

	f, err := parser.ParseFile("-", `
import "vela/base64"
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: template: spec: containers: [{
		name: "c"
		env: [for k, v in encoded {name: k, value: v}]
	}]
}
encoded: {
	KEY_0: _enc0.$returns
	KEY_1: _enc1.$returns
}
_enc0: base64.#Encode & {$params: "a"}
_enc1: base64.#Encode & {$params: "b"}
`, parser.ParseComments)
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"_enc0", "_enc1"}, []string(scopeOf(f, imports)),
		"only the calls: encoded reads their output and output reads encoded")
}

// And the other half of the same rule: a field that takes a call rather than
// reading out of one is a call, however little its own syntax says so.
func TestTheScopeTakesACopiedCall(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()

	f, err := parser.ParseFile("-", `
import "vela/base64"
#tpl: base64.#Encode & {$params: "a"}
second: #tpl
third: second
fourth: second.$returns
`, parser.ParseComments)
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"#tpl", "second", "third"}, []string(scopeOf(f, imports)),
		"second and third copy the call; fourth reads a field out of it")
}

// Whether looking the scope's fields up by name beats enumerating the root's,
// and whether it keeps beating it as the scope grows. It does both, by more
// as the scope grows, which is not what the shape of the trade suggests: one
// enumeration against one lookup per name.
func BenchmarkLookupAgainstEnumerate(b *testing.B) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()

	for _, n := range []int{1, 8, 100} {
		var s strings.Builder
		s.WriteString("import \"vela/base64\"\n")
		s.WriteString(phaseWorkload)
		for i := 0; i < n; i++ {
			fmt.Fprintf(&s, "\ncall%d: base64.#Encode & {$params: \"k-%d\"}\n", i, i)
		}
		f, err := parser.ParseFile("-", s.String(), parser.ParseComments)
		require.NoError(b, err)
		bi := build.NewContext().NewInstance("", nil)
		bi.Imports = imports
		require.NoError(b, bi.AddSyntax(f))
		value := cuecontext.New().BuildInstance(bi)
		scope := scopeOf(f, imports)
		require.Len(b, scope, n)

		b.Run(fmt.Sprintf("calls=%d/lookup", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, ok := scope.lookup(value); !ok {
					b.Fatal("declined")
				}
			}
		})
		b.Run(fmt.Sprintf("calls=%d/enumerate", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, ok := scope.enumerate(value); !ok {
					b.Fatal("declined")
				}
			}
		})
	}
}
