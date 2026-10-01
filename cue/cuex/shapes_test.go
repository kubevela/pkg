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
	"strings"
	"sync"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// The shapes a template can put a call in.
//
// Every bug the resolver has had was a shape nothing tested: a field
// written twice, a call reached through a selector, an optional hidden
// field, a step order disagreeing with file order, two calls in a pass
// returning a whole number. None of them was exotic and all of them got
// through a corpus of thirty-eight cases, because that corpus was built
// out of the shapes already thought of, which are the shapes already
// handled.
//
// So this is built the other way round, from the dimensions a template
// varies along rather than from examples: how a field is declared, how a
// call is reached, what reads the answer, what order things run in, and
// what a provider hands back. Cases are the combinations, including the
// ones that look pointless.
//
// Each case says what it expects rather than being compared to another
// resolver, because two resolvers can agree and both be wrong. Where it
// matters a case also says how many times the provider should be called,
// which is the only thing that catches a call running twice or not at
// all: both render perfectly well.

type shapeCase struct {
	// template is compiled with the counting provider below.
	template string
	// want is the value of "out", as JSON. Empty means do not look.
	want string
	// calls is how many times the provider should have run. A negative
	// number means do not look.
	calls int
	// wantErr is a substring of the error expected, if one is.
	wantErr string
}

// counter is a provider that records every call, so a case can say a call
// ran once and mean it.
type counter struct {
	mu   sync.Mutex
	seen []string
}

func (c *counter) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = nil
}

func (c *counter) record(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, s)
}

func (c *counter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen)
}

func (c *counter) order() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.seen...)
}

type echoIn struct {
	Params string `json:"$params"`
}
type echoOut struct {
	Returns string `json:"$returns"`
}
type numIn struct {
	Params string `json:"$params"`
}
type numOut struct {
	Returns struct {
		Whole    float64 `json:"whole"`
		Fraction float64 `json:"fraction"`
		Count    int     `json:"count"`
		Flag     bool    `json:"flag"`
		Empty    string  `json:"empty"`
		Raw      []byte  `json:"raw"`
	} `json:"$returns"`
}
type maybeIn struct {
	Params string `json:"$params"`
}
type maybeOut struct {
	Returns struct {
		Absent *string `json:"absent"`
		Here   string  `json:"here"`
	} `json:"$returns"`
}

func shapePackage(c *counter) cuexruntime.Package {
	pkg, err := cuexruntime.NewInternalPackage("shape", `
package shape

#Echo: {
	#do:       "echo"
	#provider: "shape"
	$params:   string
	$returns?: string
}

#Num: {
	#do:       "num"
	#provider: "shape"
	$params:   string
	$returns?: {whole: float, fraction: float, count: int, flag: bool, empty: string, raw: _}
}

#Maybe: {
	#do:       "maybe"
	#provider: "shape"
	$params:   string
	$returns?: {absent?: _, here: string}
}
`, map[string]cuexruntime.ProviderFn{
		"echo": cuexruntime.GenericProviderFn[echoIn, echoOut](
			func(_ context.Context, in *echoIn) (*echoOut, error) {
				c.record(in.Params)
				return &echoOut{Returns: in.Params}, nil
			}),
		"num": cuexruntime.GenericProviderFn[numIn, numOut](
			func(_ context.Context, in *numIn) (*numOut, error) {
				c.record(in.Params)
				out := &numOut{}
				out.Returns.Whole = 2
				out.Returns.Fraction = 1.5
				out.Returns.Count = 3
				out.Returns.Flag = true
				out.Returns.Raw = []byte("hi")
				return out, nil
			}),
		"maybe": cuexruntime.GenericProviderFn[maybeIn, maybeOut](
			func(_ context.Context, in *maybeIn) (*maybeOut, error) {
				c.record(in.Params)
				out := &maybeOut{}
				out.Returns.Here = "yes"
				return out, nil
			}),
	})
	if err != nil {
		panic(err)
	}
	return pkg
}

const shapeImport = "import \"vela/shape\"\n"

var shapeCorpus = map[string]shapeCase{
	// how a field holding a call is declared
	"declared once": {
		template: `a: shape.#Echo & {$params: "x"}
out: a.$returns`,
		want: `"x"`, calls: 1,
	},
	"declared twice, params apart from the call": {
		template: `a: shape.#Echo
a: $params: "x"
out: a.$returns`,
		want: `"x"`, calls: 1,
	},
	"declared three times": {
		template: `a: shape.#Echo
a: $params: "x"
a: {}
out: a.$returns`,
		want: `"x"`, calls: 1,
	},
	"two fields declared twice each": {
		template: `a: shape.#Echo
a: $params: "x"
b: shape.#Echo
b: $params: "y"
out: "\(a.$returns)\(b.$returns)"`,
		want: `"xy"`, calls: 2,
	},
	"hidden": {
		template: `_a: shape.#Echo & {$params: "x"}
out: _a.$returns`,
		want: `"x"`, calls: 1,
	},
	"hidden, declared twice": {
		template: `_a: shape.#Echo
_a: $params: "x"
out: _a.$returns`,
		want: `"x"`, calls: 1,
	},
	"optional": {
		template: `a?: shape.#Echo & {$params: "x"}
out: "done"`,
		want: `"done"`, calls: 1,
	},
	"optional hidden": {
		template: `_a?: shape.#Echo & {$params: "x"}
out: "done"`,
		want: `"done"`, calls: 1,
	},
	"quoted, reached by alias": {
		// a quoted label declares no identifier, so out: a.$returns does
		// not resolve in plain CUE either
		template: `X="a": shape.#Echo & {$params: "x"}
out: X.$returns`,
		want: `"x"`, calls: 1,
	},
	"quoted with a dash, reached by alias": {
		template: `X="a-b": shape.#Echo & {$params: "x"}
out: X.$returns`,
		want: `"x"`, calls: 1,
	},
	"a definition, used once": {
		template: `#tpl: shape.#Echo & {$params: "x"}
a: #tpl
out: a.$returns`,
		want: `"x"`, calls: 1,
	},
	"a definition, used twice": {
		template: `#tpl: shape.#Echo & {$params: "x"}
a: #tpl
b: #tpl
out: "\(a.$returns)\(b.$returns)"`,
		want: `"xx"`, calls: 2,
	},

	// How a call is reached.
	//
	// Copying a call makes a second call, and both of them run: the value
	// has two nodes each carrying a #do, and nothing says they are the
	// same one. Every count below is what the resolver this replaced does,
	// checked against it rather than assumed, so a change to any of them
	// is a change in behaviour and not a fix.
	//
	// This is not the same as one field written twice. That is one node,
	// and running it twice was a bug.
	"copied by name": {
		template: `_a: shape.#Echo & {$params: "x"}
b: _a
out: b.$returns`,
		want: `"x"`, calls: 2,
	},
	"copied through a selector on a definition": {
		// One, where the resolver this replaced ran two. A definition is
		// not walked, so the call under it is only found where it was
		// copied to. The output is the same and the side effect happens
		// once instead of twice, which is the better of the two, but it is
		// a difference and worth having written down.
		template: `#tpl: inner: shape.#Echo & {$params: "x"}
b: #tpl.inner
out: b.$returns`,
		want: `"x"`, calls: 1,
	},
	"the answer read back through the definition": {
		// The call ran at b, so this asks whether #tpl.inner has an answer
		// too. It does not, and the template gets an error rather than a
		// wrong value.
		template: `#tpl: inner: shape.#Echo & {$params: "x"}
b: #tpl.inner
out: #tpl.inner.$returns`,
		wantErr: "cannot reference optional field: $returns",
		calls:   1,
	},
	"copied through a selector on a field": {
		template: `holder: inner: shape.#Echo & {$params: "x"}
b: holder.inner
out: b.$returns`,
		want: `"x"`, calls: 2,
	},
	"reached through an index": {
		template: `holder: [shape.#Echo & {$params: "x"}]
b: holder[0]
out: b.$returns`,
		want: `"x"`, calls: 2,
	},
	"under a nested field": {
		template: `outer: middle: inner: shape.#Echo & {$params: "x"}
out: outer.middle.inner.$returns`,
		want: `"x"`, calls: 1,
	},
	"in a list": {
		template: `items: [shape.#Echo & {$params: "a"}, shape.#Echo & {$params: "b"}]
out: "\(items[0].$returns)\(items[1].$returns)"`,
		want: `"ab"`, calls: 2,
	},
	"the template is the call": {
		template: `#do: "echo"
#provider: "shape"
$params: "x"`,
		calls: 1,
	},

	// what reads the answer
	"read whole": {
		template: `a: shape.#Echo & {$params: "x"}
out: a.$returns`,
		want: `"x"`, calls: 1,
	},
	"read by interpolation": {
		template: `a: shape.#Echo & {$params: "x"}
out: "<\(a.$returns)>"`,
		want: `"<x>"`, calls: 1,
	},
	"params read back": {
		template: `a: shape.#Echo & {$params: "x"}
out: a.$params`,
		want: `"x"`, calls: 1,
	},
	"answer feeds another call": {
		template: `a: shape.#Echo & {$params: "x"}
b: shape.#Echo & {$params: a.$returns}
out: b.$returns`,
		want: `"x"`, calls: 2,
	},
	"answer read twice": {
		template: `a: shape.#Echo & {$params: "x"}
out: "\(a.$returns)\(a.$returns)"`,
		want: `"xx"`, calls: 1,
	},

	// what a provider hands back, alone and in company
	"a whole float alone": {
		template: `a: shape.#Num & {$params: "x"}
out: a.$returns.whole`,
		want: `2`, calls: 1,
	},
	"a whole float in company": {
		template: `a: shape.#Num & {$params: "x"}
b: shape.#Num & {$params: "y"}
out: a.$returns.whole`,
		want: `2`, calls: 2,
	},
	"a fraction in company": {
		template: `a: shape.#Num & {$params: "x"}
b: shape.#Num & {$params: "y"}
out: a.$returns.fraction`,
		want: `1.5`, calls: 2,
	},
	"an int in company": {
		template: `a: shape.#Num & {$params: "x"}
b: shape.#Num & {$params: "y"}
out: a.$returns.count`,
		want: `3`, calls: 2,
	},
	"a bool in company": {
		template: `a: shape.#Num & {$params: "x"}
b: shape.#Num & {$params: "y"}
out: a.$returns.flag`,
		want: `true`, calls: 2,
	},
	"an empty string in company": {
		template: `a: shape.#Num & {$params: "x"}
b: shape.#Num & {$params: "y"}
out: a.$returns.empty`,
		want: `""`, calls: 2,
	},
	"an unset pointer alone": {
		template: `a: shape.#Maybe & {$params: "x"}
out: a.$returns.here`,
		want: `"yes"`, calls: 1,
	},
	"an unset pointer in company": {
		template: `a: shape.#Maybe & {$params: "x"}
b: shape.#Maybe & {$params: "y"}
out: a.$returns.here`,
		want: `"yes"`, calls: 2,
	},

	// comprehensions
	"a loop of calls": {
		template: `import "list"
_idx: list.Range(0, 3, 1)
_c: {for i in _idx {"\(i)": shape.#Echo & {$params: "s\(i)"}}}
out: {for i in _idx {"\(i)": _c["\(i)"].$returns}}`,
		want: `{"0":"s0","1":"s1","2":"s2"}`, calls: 3,
	},
	"a loop with a condition": {
		template: `import "list"
_idx: list.Range(0, 4, 1)
_c: {for i in _idx if i mod 2 == 0 {"\(i)": shape.#Echo & {$params: "s\(i)"}}}
out: {for i in _idx if i mod 2 == 0 {"\(i)": _c["\(i)"].$returns}}`,
		want: `{"0":"s0","2":"s2"}`, calls: 2,
	},
	"a loop over a struct": {
		template: `_src: {a: "1", b: "2"}
_c: {for k, v in _src {"\(k)": shape.#Echo & {$params: v}}}
out: {for k, _ in _src {"\(k)": _c[k].$returns}}`,
		want: `{"a":"1","b":"2"}`, calls: 2,
	},
	"a loop inside a loop": {
		template: `import "list"
_o: list.Range(0, 2, 1)
_c: {for a in _o {for b in _o {"\(a)\(b)": shape.#Echo & {$params: "\(a)\(b)"}}}}
out: {for a in _o for b in _o {"\(a)\(b)": _c["\(a)\(b)"].$returns}}`,
		want: `{"00":"00","01":"01","10":"10","11":"11"}`, calls: 4,
	},
	"a loop whose answers feed a second loop": {
		template: `import "list"
_idx: list.Range(0, 2, 1)
_first: {for i in _idx {"\(i)": shape.#Echo & {$params: "a\(i)"}}}
_second: {for i in _idx {"\(i)": shape.#Echo & {$params: _first["\(i)"].$returns}}}
out: {for i in _idx {"\(i)": _second["\(i)"].$returns}}`,
		want: `{"0":"a0","1":"a1"}`, calls: 4,
	},

	// let, alias and embedding
	"a let feeding a call": {
		template: `let p = "x"
a: shape.#Echo & {$params: p}
out: a.$returns`,
		want: `"x"`, calls: 1,
	},
	"an alias on the field holding a call": {
		template: `X=a: shape.#Echo & {$params: "x"}
out: X.$returns`,
		want: `"x"`, calls: 1,
	},
	"a struct holding a call, embedded": {
		template: `_base: inner: shape.#Echo & {$params: "x"}
wrap: {_base}
out: wrap.inner.$returns`,
		want: `"x"`, calls: 2,
	},

	// what runs first
	"step order agreeing with file order": {
		template: `a: shape.#Echo & {$params: "a"} @step(1)
b: shape.#Echo & {$params: "b"} @step(2)
out: "\(a.$returns)\(b.$returns)"`,
		want: `"ab"`, calls: 2,
	},
	"step order disagreeing with file order": {
		template: `b: shape.#Echo & {$params: "b"} @step(2)
a: shape.#Echo & {$params: "a"} @step(1)
out: "\(a.$returns)\(b.$returns)"`,
		want: `"ab"`, calls: 2,
	},
	"step order on hidden fields": {
		template: `_b: shape.#Echo & {$params: "b"} @step(2)
_a: shape.#Echo & {$params: "a"} @step(1)
out: "\(_a.$returns)\(_b.$returns)"`,
		want: `"ab"`, calls: 2,
	},

	// nothing to do
	"no calls at all": {
		template: `out: "plain"`,
		want:     `"plain"`, calls: 0,
	},
	"a field named like a call but not one": {
		template: `a: {"#do": "echo", "$params": "x"}
out: a["#do"]`,
		want: `"echo"`, calls: 0,
	},
}

func TestShapesACallCanBeIn(t *testing.T) {
	c := &counter{}
	compiler := cuex.NewCompilerWithInternalPackages(shapePackage(c))

	names := make([]string, 0, len(shapeCorpus))
	for name := range shapeCorpus {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		tc := shapeCorpus[name]
		t.Run(name, func(t *testing.T) {
			c.reset()
			src := tc.template + "\n"
			if strings.Contains(src, "shape.#") {
				src = shapeImport + src
			}
			v, err := compiler.CompileString(context.Background(), src)

			// A template can fail at either end: the resolve can report
			// it, or the value can come back holding a bottom that only
			// shows when something reads it. A case saying it expects an
			// error should not have to know which.
			readErr := err
			var out []byte
			if err == nil {
				out, readErr = v.LookupPath(cue.ParsePath("out")).MarshalJSON()
			}
			if tc.wantErr != "" {
				require.Error(t, readErr)
				require.Contains(t, readErr.Error(), tc.wantErr)
				// A case that errors still says what ran. Without this
				// the comment on a case like "the call ran at b" is a
				// claim the case cannot check.
				if tc.calls >= 0 {
					require.Equal(t, tc.calls, c.count(),
						"calls made: %v", c.order())
				}
				return
			}
			require.NoError(t, err)
			if tc.want != "" {
				require.NoError(t, readErr, "reading out")
				require.JSONEq(t, tc.want, string(out))
			}
			if tc.calls >= 0 {
				require.Equal(t, tc.calls, c.count(),
					"the provider ran %d time(s), wanted %d: %v",
					c.count(), tc.calls, c.order())
			}
		})
	}
}

// Step attributes say what runs first, and a case that only checks the
// answers cannot tell whether they did.
func TestShapesRunInStepOrder(t *testing.T) {
	c := &counter{}
	compiler := cuex.NewCompilerWithInternalPackages(shapePackage(c))

	for _, tc := range []struct {
		name, template string
		want           []string
	}{
		{
			name: "file order, no steps",
			template: `a: shape.#Echo & {$params: "a"}
b: shape.#Echo & {$params: "b"}
out: "\(a.$returns)\(b.$returns)"`,
			want: []string{"a", "b"},
		},
		{
			name: "steps disagreeing with file order",
			template: `b: shape.#Echo & {$params: "b"} @step(2)
a: shape.#Echo & {$params: "a"} @step(1)
out: "\(a.$returns)\(b.$returns)"`,
			want: []string{"a", "b"},
		},
		{
			name: "three steps, written backwards",
			template: `c: shape.#Echo & {$params: "c"} @step(3)
b: shape.#Echo & {$params: "b"} @step(2)
a: shape.#Echo & {$params: "a"} @step(1)
out: "\(a.$returns)\(b.$returns)\(c.$returns)"`,
			want: []string{"a", "b", "c"},
		},
		{
			name: "hidden fields, steps backwards",
			template: `_c: shape.#Echo & {$params: "c"} @step(3)
_a: shape.#Echo & {$params: "a"} @step(1)
_b: shape.#Echo & {$params: "b"} @step(2)
out: "\(_a.$returns)\(_b.$returns)\(_c.$returns)"`,
			want: []string{"a", "b", "c"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c.reset()
			_, err := compiler.CompileString(context.Background(),
				shapeImport+tc.template+"\n")
			require.NoError(t, err)
			require.Equal(t, tc.want, c.order())
		})
	}
}
