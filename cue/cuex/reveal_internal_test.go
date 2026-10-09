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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"
)

// Which calls can reveal another is read from the syntax. A call reveals where
// a comprehension or a computed label that can produce a call reads its result,
// directly or through fields that do; nothing else does.
func TestRevealers(t *testing.T) {
	for name, tc := range map[string]struct {
		src     string
		reveal  []string
		inPlace []string
	}{
		"a guard on a result": {`
check: rec.#Do & {$params: "x"}
if check.$returns == "failed" {
	fail: rec.#Do & {$params: "fail"}
}
wait: rec.#Do & {$params: "w"}`, []string{"check"}, []string{"wait", "fail"}},
		"a guard on a field naming a result": {`
check:  rec.#Do & {$params: "x"}
result: check.$returns
if result == "failed" {
	fail: rec.#Do & {$params: "fail"}
}`, []string{"check"}, []string{"fail"}},
		"a loop over a result": {`
list: rec.#Do & {$params: "x"}
each: {for i, v in list.$returns {"\(i)": rec.#Do & {$params: v}}}`, []string{"list"}, []string{`each."0"`}},
		"a label computed from a result": {`
check: rec.#Do & {$params: "x"}
"\(check.$returns)": rec.#Do & {$params: "y"}`, []string{"check"}, nil},
		"a nested call under a field the guard reads": {`
bind: {load: rec.#Do & {$params: "x"}}
if bind.load.$returns != _|_ {
	fail: rec.#Do & {$params: "fail"}
}`, []string{"bind.load"}, []string{"fail"}},
		"a guard on an alias of a result": {`
check: rec.#Do & {$params: "x"}
R=result: check.$returns
if R == "failed" {
	fail: rec.#Do & {$params: "fail"}
}`, []string{"check"}, []string{"fail"}},
		"a guard on a let naming a result": {`
check: rec.#Do & {$params: "x"}
let r = check.$returns
if r == "failed" {
	fail: rec.#Do & {$params: "fail"}
}`, []string{"check"}, []string{"fail"}},
		// the body names its call rather than writing it
		"a guard writing a call a let holds": {`
check: rec.#Do & {$params: "x"}
let next = rec.#Do & {$params: "n"}
if check.$returns == "failed" {
	out: next
}`, []string{"check"}, nil},
		"a guard copying a field that holds a call, through another": {`
check:  rec.#Do & {$params: "x"}
spec:   rec.#Do & {$params: "n"}
copied: spec
if check.$returns == "failed" {
	out: copied
}`, []string{"check"}, nil},
		"a guard on a definition naming a result": {`
check:   rec.#Do & {$params: "x"}
#result: check.$returns
if #result == "failed" {
	fail: rec.#Do & {$params: "fail"}
}`, []string{"check"}, []string{"fail"}},
		"a guard on a hidden definition naming a result": {`
check:    rec.#Do & {$params: "x"}
_#result: check.$returns
if _#result == "failed" {
	fail: rec.#Do & {$params: "fail"}
}`, []string{"check"}, []string{"fail"}},
		// every call has $params and $returns: sharing those names, or reading
		// a result some guard elsewhere also reads, does not make a call
		// reveal one
		"a call beside a guard on another": {`
check: rec.#Do & {$params: "x"}
if check.$returns == "failed" {
	fail: rec.#Do & {$params: "fail"}
}
probe: rec.#Do & {$params: "y"}
msg:   probe.$returns
next:  rec.#Do & {$params: msg}`, []string{"check"}, []string{"probe", "next", "fail"}},
		// the fanout writes calls, but over a list no call produces
		"a loop over a list no call produces": {`
idx: [0, 1, 2]
fan: {for i in idx {"\(i)": rec.#Do & {$params: "\(i)"}}}
after: rec.#Do & {$params: "y"}`, nil, []string{`fan."0"`, "after"}},
		// a comprehension over results that writes data, not calls
		"a gather that writes no call": {`
a: rec.#Do & {$params: "x"}
b: rec.#Do & {$params: "y"}
gathered: [for r in [a, b] {r.$returns}]`, nil, []string{"a", "b"}},
		"a guard that writes no call": {`
check: rec.#Do & {$params: "x"}
if check.$returns == "failed" {
	message: "it failed"
}`, nil, []string{"check"}},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseFile("-", "import \"vela/rec\"\n"+tc.src)
			require.NoError(t, err)
			r := readReveals(f).close()
			for _, path := range tc.reveal {
				require.True(t, r.after(pendingCall{path: cue.ParsePath(path)}), "%s reveals", path)
			}
			for _, path := range tc.inPlace {
				require.False(t, r.after(pendingCall{path: cue.ParsePath(path)}), "%s reveals nothing", path)
			}
		})
	}
}

// Where the syntax cannot say what is in the value, every call reveals.
func TestRevealersWithoutSyntax(t *testing.T) {
	require.True(t, (&revealers{all: true}).after(pendingCall{path: cue.ParsePath("any")}))
	f, err := parser.ParseFile("-", `x: 1`)
	require.NoError(t, err)
	in := NewCompilerWithInternalPackages()
	r := in.revealersOf(f, NewCompileConfig(WithData("x", cue.Value{})), nil)
	require.True(t, r.all, "a filled cue.Value can carry a call")
	r = in.revealersOf(f, NewCompileConfig(), nil)
	require.False(t, r.all)
}

// Calls that reveal nothing share a round: a fanout over a list no call
// produces runs in one, rather than one call per search of the value.
func TestCallsThatRevealNothingShareARound(t *testing.T) {
	rec := &recorder{}
	in := NewCompilerWithInternalPackages(rec.pkg())
	src := `
import "vela/rec"
idx: [0, 1, 2, 3]
fan: {for i in idx {"\(i)": rec.#Do & {$params: "n\(i)"}}}
`
	value, err := in.CompileStringWithOptions(context.Background(), src, DisableResolveProviderFunctions{})
	require.NoError(t, err)
	f, err := parser.ParseFile("-", src)
	require.NoError(t, err)
	reveal := in.revealersOf(f, NewCompileConfig(), in.PackageManager.GetImports())

	executed := map[string]bool{}
	pending := pendingCalls(value, executed, nil)
	require.Len(t, pending, 4)
	_, _, err = in.runRound(context.Background(), value, in.PackageManager.GetProviders(), pending, executed,
		&callWaits{of: map[string][]string{}}, reveal)
	require.NoError(t, err)
	require.Len(t, executed, 4, "every call ran in the one round")
	require.Equal(t, []string{"n0", "n1", "n2", "n3"}, rec.order())
}
