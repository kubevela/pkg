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
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
)

// A hidden field is where a definition puts a call whose answer it does not
// want in the rendered output, so it is the common shape rather than an edge
// of one. A pass with more than one result collects them as syntax and
// unifies once, and a hidden name has to be written as an identifier there: a
// quoted "_h" is a different field from _h, and a result written that way
// lands beside the call rather than in it.
func TestHiddenCallsInOneRound(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	v, err := c.CompileString(context.Background(), `
import "vela/base64"
_first:  base64.#Encode & {$params: "one"}
_second: base64.#Encode & {$params: "two"}
_third:  base64.#Encode & {$params: "three"}
out: {
	a: _first.$returns
	b: _second.$returns
	c: _third.$returns
}
`)
	require.NoError(t, err)

	for path, want := range map[string]string{
		"out.a": "b25l",     // one
		"out.b": "dHdv",     // two
		"out.c": "dGhyZWU=", // three
	} {
		got, err := v.LookupPath(cue.ParsePath(path)).String()
		require.NoError(t, err, "%s", path)
		require.Equal(t, want, got, "%s", path)
	}

	// and nothing landed in a quoted field beside the hidden one
	bs, err := v.MarshalJSON()
	require.NoError(t, err)
	require.NotContains(t, string(bs), `"_first"`,
		"a quoted _first would mean the answer went next to the call rather than into it")
}

// A hidden name and a quoted one that spell the same thing are different
// fields, and the collection has to keep them apart. Keyed by the name alone
// they merge, and one of the two answers then goes to the wrong field with
// nothing said about it.
func TestHiddenAndQuotedSameSpelling(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	v, err := c.CompileString(context.Background(), `
import "vela/base64"
a: {
	"_h": {x: base64.#Encode & {$params: "quoted"}}
	_h:   {y: base64.#Encode & {$params: "hidden"}}
	fromHidden: _h.y.$returns
}
fromQuoted: a["_h"].x.$returns
`)
	require.NoError(t, err)

	got, err := v.LookupPath(cue.ParsePath("a.fromHidden")).String()
	require.NoError(t, err, "the hidden call's answer went somewhere else")
	require.Equal(t, "aGlkZGVu", got)

	got, err = v.LookupPath(cue.ParsePath("fromQuoted")).String()
	require.NoError(t, err)
	require.Equal(t, "cXVvdGVk", got)
}

// A template can be the call itself, with its #do at the root rather than
// under a field. The scope names fields, so it has no name for that one and
// has to stand aside rather than report a template with no calls in it.
func TestTheTemplateIsTheCall(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	v, err := c.CompileString(context.Background(), `
#do:       "encode"
#provider: "base64"
$params:   "root"
`)
	require.NoError(t, err)
	got, err := v.LookupPath(cue.ParsePath("$returns")).String()
	require.NoError(t, err, "a call at the root is still a call")
	require.Equal(t, "cm9vdA==", got)
}

// Hidden calls used to fall out of the overlay and be filled one at a time,
// which is a unify each and quadratic in the number of them. This is the
// shape that showed it: the same fanout, named two ways.
func TestHiddenCallsCostWhatPlainOnesDo(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	build := func(prefix string, n int) string {
		var b strings.Builder
		b.WriteString("import \"vela/base64\"\n")
		b.WriteString("out: {\n")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "\tk%d: %s%d.$returns\n", i, prefix, i)
		}
		b.WriteString("}\n")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "%s%d: base64.#Encode & {$params: \"v-%d\"}\n", prefix, i, i)
		}
		return b.String()
	}

	const n = 40
	hidden, err := c.CompileString(ctx, build("_enc", n))
	require.NoError(t, err)
	plain, err := c.CompileString(ctx, build("enc", n))
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		path := cue.ParsePath(fmt.Sprintf("out.k%d", i))
		h, err := hidden.LookupPath(path).String()
		require.NoError(t, err)
		p, err := plain.LookupPath(path).String()
		require.NoError(t, err)
		require.Equal(t, p, h, "the two namings have to answer the same")
	}
}
