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
	"context"
	"fmt"
	"testing"

	"cuelang.org/go/cue/format"
	"github.com/stretchr/testify/require"
)

// The prepass against the resolver, on the same template, asking for the
// same thing: does a template whose loops were answered beforehand render
// to what the resolver renders?
//
// The prepass is allowed to decline, and a case that declines is reported
// rather than passed over, because a prepass that quietly does nothing
// looks exactly like one that works.

func loopSrc(n int) string {
	return fmt.Sprintf(`
import "vela/base64"
import "list"
_idx: list.Range(0, %d, 1)
_calls: {
	for i in _idx {
		"\(i)": base64.#Encode & {$params: "seed-\(i)"}
	}
}
out: {for i in _idx {"\(i)": _calls["\(i)"].$returns}}
`, n)
}

func TestPrepassRendersWhatTheResolverRenders(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	imports := c.PackageManager.GetImports()
	policy := OptimisePolicy{Enabled: true, Threshold: 1}

	for _, n := range []int{1, 5, 50} {
		src := loopSrc(n)

		// what the resolver makes of it
		want, err := c.CompileString(ctx, src)
		require.NoError(t, err)
		wantJSON, err := want.MarshalJSON()
		require.NoError(t, err)

		// and what the prepass leaves for it to make
		got, took := c.prepass(ctx, src, imports, policy)
		require.True(t, took, "width %d: the prepass should have taken this loop", n)
		require.Equal(t, 1, got.loops)
		require.Equal(t, n, got.calls)

		bs, err := format.Node(got.file)
		require.NoError(t, err)
		if n == 1 {
			t.Logf("the template the prepass hands on:\n%s", bs)
		}
		require.NotContains(t, string(bs), "base64.#Encode",
			"the calls should be answered, not carried")

		after, err := c.CompileString(ctx, string(bs))
		require.NoError(t, err, "width %d", n)
		afterJSON, err := after.MarshalJSON()
		require.NoError(t, err)

		require.JSONEq(t, string(wantJSON), string(afterJSON),
			"width %d: a prepassed template must render to what the resolver renders", n)
		t.Logf("RUN width=%-4d %d calls answered beforehand, render matches", n, got.calls)
	}
}

// What the prepass refuses, and that it refuses rather than guessing. Each
// of these leaves the template exactly as it was, for the resolver.
//
// A loop inside a loop is not among them any more. It used to be refused
// because one iteration produced more than one field, and answering a run
// of iterations at a time means that is the ordinary case: the inner loop
// expands for each of the outer ones the run pins, and every field it
// makes is answered by the key it landed under. The loop corpus holds it
// to rendering what the resolver renders.
func TestPrepassDeclines(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	imports := c.PackageManager.GetImports()
	open := OptimisePolicy{Enabled: true, Threshold: 1}

	for _, tc := range []struct {
		name, src string
		policy    OptimisePolicy
	}{
		{
			name:   "a loop under the threshold",
			src:    loopSrc(5),
			policy: OptimisePolicy{Enabled: true, Threshold: 50},
		},
		{
			name:   "the prepass turned off",
			src:    loopSrc(5),
			policy: OptimisePolicy{},
		},
		{
			name: "a loop binding a key",
			src: `
import "vela/base64"
_src: {a: "x", b: "y"}
_calls: {
	for k, v in _src {
		"\(k)": base64.#Encode & {$params: v}
	}
}
`,
			policy: open,
		},
		{
			name: "a loop that makes something that is not a call",
			src: `
import "list"
_idx: list.Range(0, 3, 1)
_calls: {
	for i in _idx {
		"\(i)": {plain: i}
	}
}
`,
			policy: open,
		},
		{
			name: "a name the file does not declare",
			src: `
import "vela/base64"
import "list"
_idx: list.Range(0, 3, 1)
_calls: {
	for i in _idx {
		"\(i)": base64.#Encode & {$params: "\(missing)-\(i)"}
	}
}
`,
			policy: open,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, took := c.prepass(ctx, tc.src, imports, tc.policy)
			require.False(t, took, "this should have been left to the resolver")
		})
	}
}

// A template with no loop in it is not slowed down by being offered to the
// prepass.
func TestPrepassLeavesAPlainTemplateAlone(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	src := "a: \"plain\"\nb: {c: 1}\n"
	_, took := c.prepass(context.Background(), src,
		c.PackageManager.GetImports(), DefaultOptimisePolicy)
	require.False(t, took)
}
