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
package cel

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cel/template"
)

func readsOf(t *testing.T, expr string) []string {
	t.Helper()
	refs, err := testEngine(t).PropertyReferences(expr)
	require.NoError(t, err, expr)
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		s := r.String()
		if r.Defaulted {
			s += " (guarded)"
		}
		out = append(out, s)
	}
	return out
}

func TestReadsKeepALiteralIndex(t *testing.T) {
	refs, err := testEngine(t).PropertyReferences(`alpha.cfg.ports[1]`)
	require.NoError(t, err)
	require.Equal(t, []template.Reference{{Root: "alpha", Path: []string{"cfg", "ports", "1"}}}, refs)
}

// With a computed index, the container is what is read; nothing below it is
// known before evaluation.
func TestAComputedIndexReadsItsContainer(t *testing.T) {
	require.Equal(t, []string{"alpha.cfg.items", "alpha.i"}, readsOf(t, `alpha.cfg.items[alpha.i].name`))
}

// A parent read written alongside a child read is a read in its own right.
func TestAParentReadIsKeptBesideItsChild(t *testing.T) {
	require.Equal(t, []string{"alpha.cfg", "alpha.cfg.host"}, readsOf(t, `alpha.cfg == null ? "" : alpha.cfg.host`))
}

// An iteration variable named like a root is local to the comprehension.
func TestAnIterationVariableShadowsARoot(t *testing.T) {
	require.Empty(t, readsOf(t, `["x"].map(alpha, alpha)`))
	require.Equal(t, []string{"alpha.list"}, readsOf(t, `alpha.list.map(alpha, alpha.x)`))
}

func TestAGuardMustProveThePathForItsArm(t *testing.T) {
	for expr, want := range map[string]string{
		`has(alpha.x) ? alpha.x : "fb"`:                 "alpha.x (guarded)",
		`!has(alpha.x) ? "fb" : alpha.x`:                "alpha.x (guarded)",
		`has(alpha.x) ? "fb" : alpha.x`:                 "alpha.x",
		`(has(alpha.x) || alpha.y) ? alpha.x : "fb"`:    "alpha.x",
		`has(alpha.z) && has(alpha.x) ? alpha.x : "fb"`: "alpha.x (guarded)",
		`has(alpha.x) && alpha.x == "a"`:                "alpha.x (guarded)",
		`!has(alpha.x) || alpha.x == "a"`:               "alpha.x (guarded)",
		`has(alpha.x) || alpha.x == "a"`:                "alpha.x",
	} {
		require.Contains(t, readsOf(t, expr), want, expr)
	}
}

func TestAQualifierArgumentMustBeALiteral(t *testing.T) {
	_, err := testEngine(t).PropertyReferences(`peer.db.at(alpha.region).host`)
	require.ErrorContains(t, err, "at() takes a literal string")
}

// NUL marks a qualifier in a read path, so only a key that becomes part of one
// is refused; a NUL in an ordinary value is fine.
func TestNULIsRefusedOnlyInAReadKey(t *testing.T) {
	require.Equal(t, []string{"alpha.v"}, readsOf(t, "alpha.v == \"a\\u0000b\""))
	_, err := testEngine(t).PropertyReferences("alpha.m[\"a\\u0000b\"]")
	require.ErrorContains(t, err, "NUL")
}
