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

package template

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsIndexSegment(t *testing.T) {
	require.True(t, isIndexSegment("0"))
	require.True(t, isIndexSegment("42"))
	require.False(t, isIndexSegment(""))
	require.False(t, isIndexSegment("name"))
	require.False(t, isIndexSegment("1a"), "a field cannot start with a digit, so this is a name")
	require.False(t, isIndexSegment("-1"))
}

func TestIsIdent(t *testing.T) {
	require.True(t, isIdent("name"))
	require.True(t, isIdent("_name"))
	require.False(t, isIdent("$name"), "not an identifier in CEL")
	require.True(t, isIdent("n4me"))
	require.False(t, isIdent(""))
	require.False(t, isIdent("4name"), "cannot start with a digit")
	require.False(t, isIdent("my-name"), "a hyphen needs bracket syntax")
	require.False(t, isIdent("a.b"))
}

// The rendered form has to be an expression the author can paste, because the
// errors using it say "supply a default with *<ref> | <fallback>".
func TestReferenceStringRendersSomethingThatParses(t *testing.T) {
	for _, tc := range []struct {
		ref  Reference
		want string
	}{
		{ref("source", "cfg", "host"), `source.cfg.host`},
		{ref("source", "cfg", "items", "0", "name"), `source.cfg.items[0].name`},
		{ref("source", "my-source", "host"), `source["my-source"].host`},
		{ref("context", "appLabels", "a.b/c"), `context.appLabels["a.b/c"]`},
		{ref("context", "cluster"), `context.cluster`},
		{ref("source", "cfg", "$ref"), `source.cfg["$ref"]`},
		{ref("source", "cfg", "in"), `source.cfg["in"]`},
		{ref("source", "cfg", "null"), `source.cfg["null"]`},
	} {
		require.Equal(t, tc.want, tc.ref.String())
	}
}

func ref(root string, path ...string) Reference { return Reference{Root: root, Path: path} }
