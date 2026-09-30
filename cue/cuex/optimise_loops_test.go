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
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
)

// Loops, which the resolve corpus has almost none of.
//
// Running the prepass over that corpus declined all 41 cases, so it said
// nothing about whether the prepass is right, only that it is careful.
// These are templates built to be taken: every way a definition can put a
// provider call inside a comprehension, plus the ways it can put one there
// that the prepass should refuse.
//
// The bar is the same. Render to what the resolver renders, or decline.

var loopCorpus = map[string]string{
	"a plain loop of calls": `
import "vela/base64"
import "list"
_idx: list.Range(0, 4, 1)
_calls: {for i in _idx {"\(i)": base64.#Encode & {$params: "seed-\(i)"}}}
out: {for i in _idx {"\(i)": _calls["\(i)"].$returns}}`,

	"a loop reading another field": `
import "vela/base64"
import "list"
_idx:    list.Range(0, 4, 1)
_prefix: "p"
_calls: {for i in _idx {"\(i)": base64.#Encode & {$params: "\(_prefix)-\(i)"}}}
out: {for i in _idx {"\(i)": _calls["\(i)"].$returns}}`,

	"a loop with a let in it": `
import "vela/base64"
import "list"
_idx: list.Range(0, 4, 1)
_calls: {for i in _idx {let d = i * 3, "\(i)": base64.#Encode & {$params: "n-\(d)"}}}
out: {for i in _idx {"\(i)": _calls["\(i)"].$returns}}`,

	"a loop with a condition": `
import "vela/base64"
import "list"
_idx: list.Range(0, 6, 1)
_calls: {for i in _idx if i mod 2 == 0 {"\(i)": base64.#Encode & {$params: "e-\(i)"}}}
out: {for i in _idx if i mod 2 == 0 {"\(i)": _calls["\(i)"].$returns}}`,

	"a loop over a list of strings": `
import "vela/base64"
_names: ["alpha", "beta", "gamma"]
_calls: {for n in _names {"\(n)": base64.#Encode & {$params: n}}}
out: {for n in _names {"\(n)": _calls["\(n)"].$returns}}`,

	"a loop whose source is a field built from another": `
import "vela/base64"
import "list"
_count: 3
_idx:   list.Range(0, _count, 1)
_calls: {for i in _idx {"\(i)": base64.#Encode & {$params: "s-\(i)"}}}
out: {for i in _idx {"\(i)": _calls["\(i)"].$returns}}`,

	"a loop reading a field declared twice": `
import "vela/base64"
import "list"
_idx: list.Range(0, 3, 1)
cfg: {a: "x"}
cfg: {b: "y"}
_calls: {for i in _idx {"\(i)": base64.#Encode & {$params: "\(cfg.a)\(cfg.b)-\(i)"}}}
out: {for i in _idx {"\(i)": _calls["\(i)"].$returns}}`,

	"a loop reading parameter": `
import "vela/base64"
import "list"
parameter: {tag: string | *"v1"}
_idx: list.Range(0, 3, 1)
_calls: {for i in _idx {"\(i)": base64.#Encode & {$params: "\(parameter.tag)-\(i)"}}}
out: {for i in _idx {"\(i)": _calls["\(i)"].$returns}}`,

	"two loops, one feeding the other": `
import "vela/base64"
import "list"
import "strings"
_idx: list.Range(0, 3, 1)
_first:  {for i in _idx {"\(i)": base64.#Encode & {$params: "a-\(i)"}}}
_second: {for i in _idx {"\(i)": base64.#Encode & {$params: strings.SliceRunes(_first["\(i)"].$returns, 0, 4)}}}
out: {for i in _idx {"\(i)": _second["\(i)"].$returns}}`,

	"a loop whose answers are read by name": `
import "vela/base64"
_names: ["one", "two"]
_calls: {for n in _names {"\(n)": base64.#Encode & {$params: n}}}
out: {first: _calls.one.$returns, second: _calls.two.$returns}`,

	"a visible field holding the loop": `
import "vela/base64"
import "list"
_idx: list.Range(0, 3, 1)
calls: {for i in _idx {"\(i)": base64.#Encode & {$params: "v-\(i)"}}}
out: {for i in _idx {"\(i)": calls["\(i)"].$returns}}`,

	// the ones it should refuse
	"a loop binding a key": `
import "vela/base64"
_src: {a: "x", b: "y"}
_calls: {for k, v in _src {"\(k)": base64.#Encode & {$params: v}}}
out: {for k, _ in _src {"\(k)": _calls[k].$returns}}`,

	"a loop inside a loop": `
import "vela/base64"
import "list"
_o: list.Range(0, 2, 1)
_i: list.Range(0, 2, 1)
_calls: {for a in _o {for b in _i {"\(a)-\(b)": base64.#Encode & {$params: "\(a)\(b)"}}}}
out: {for a in _o for b in _i {"\(a)-\(b)": _calls["\(a)-\(b)"].$returns}}`,

	"a loop making more than a call each time": `
import "vela/base64"
import "list"
_idx: list.Range(0, 3, 1)
_calls: {for i in _idx {
	"c\(i)": base64.#Encode & {$params: "m-\(i)"}
	"n\(i)": i
}}
out: {for i in _idx {"\(i)": _calls["c\(i)"].$returns}}`,

	"a loop making no call at all": `
import "list"
_idx: list.Range(0, 3, 1)
_plain: {for i in _idx {"\(i)": {n: i}}}
out: {for i in _idx {"\(i)": _plain["\(i)"].n}}`,

	"a loop whose source is not known until the render": `
import "vela/base64"
import "list"
parameter: {n: int | *3}
_idx: list.Range(0, parameter.n, 1)
_calls: {for i in _idx {"\(i)": base64.#Encode & {$params: "d-\(i)"}}}
out: {for i in _idx {"\(i)": _calls["\(i)"].$returns}}`,
}

func TestPrepassAgainstLoops(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	names := make([]string, 0, len(loopCorpus))
	for name := range loopCorpus {
		names = append(names, name)
	}
	sort.Strings(names)

	var taken, declined, differed int
	for _, name := range names {
		src := loopCorpus[name]
		before := renderOf(t, c, src)
		require.NotContains(t, before, "ERR:",
			"%s: the template itself should render, or the case is testing nothing", name)

		rewritten, calls, took := c.PrepassWideOpenForTest(ctx, src)
		if !took {
			declined++
			t.Logf("LOOPS %-48s declined", name)
			continue
		}
		after := renderOf(t, c, rewritten)
		if before != after {
			differed++
			t.Errorf("LOOPS %s renders differently after the prepass\n  before: %s\n  after:  %s",
				name, before, after)
			continue
		}
		taken++
		t.Logf("LOOPS %-48s took %3d call(s), same render", name, calls)
	}

	t.Logf("LOOPS --- %d taken, %d declined, %d differed, of %d",
		taken, declined, differed, len(names))
	require.Zero(t, differed)
	require.NotZero(t, taken, "if it takes nothing, this proves nothing")
}
