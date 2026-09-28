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
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// TestPartitionDataRoutes checks which path each value takes. Without it the
// equivalence tests would still pass with everything falling back to source,
// which is the slow path they exist to avoid.
func TestPartitionDataRoutes(t *testing.T) {
	for name, tt := range map[string]struct {
		template string
		key      string
		wantFill bool
		reparsed bool
	}{
		"declared at top level": {`parameter: {...}`, "parameter", true, false},
		"declared with a value": {`parameter: input: string`, "parameter", true, false},
		"quoted string label":   {`"my-param": {...}`, `"my-param"`, true, false},
		// a bare hyphenated key is not a CUE path - it parses as subtraction -
		// so it cannot be filled. WithExtraData cannot address it either.
		"hyphenated key, unquoted":     {`"my-param": {...}`, "my-param", false, true},
		"dotted key, root declared":    {`parameter: {...}`, "parameter.nested", true, false},
		"not declared":                 {`a: parameter`, "parameter", false, true},
		"dotted key, root undeclared":  {`a: parameter.nested`, "parameter.nested", false, true},
		"declared only inside another": {`wrap: parameter: {...}`, "parameter", false, true},
		"declared as a definition":     {`#parameter: {...}`, "parameter", false, true},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseFile("-", tt.template, parser.ParseComments)
			require.NoError(t, err)
			fills, out, _, err := partitionData([]*withData{{key: tt.key, data: map[string]any{"k": "v"}}}, tt.template, f)
			require.NoError(t, err)
			if tt.wantFill {
				require.Len(t, fills, 1, "value should be filled into the built value")
			} else {
				require.Empty(t, fills, "value should have fallen back to source")
			}
			if tt.reparsed {
				require.NotSame(t, f, out, "falling back has to parse the enlarged source")
			} else {
				require.Same(t, f, out, "the file should not be parsed twice")
			}
		})
	}
}

func TestPartitionDataNoValues(t *testing.T) {
	f, err := parser.ParseFile("-", `a: 1`, parser.ParseComments)
	require.NoError(t, err)
	fills, out, _, err := partitionData(nil, `a: 1`, f)
	require.NoError(t, err)
	require.Empty(t, fills)
	require.Same(t, f, out)
}

// TestMayContainCalls checks when resolution is skipped. Skipping wrongly
// means a provider call never runs and nothing says so, which is why the
// answer is yes whenever the question cannot be settled from here.
func TestMayContainCalls(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	for name, tt := range map[string]struct {
		src  string
		opts []CompileOption
		want bool
	}{
		"plain template":              {`out: {a: "1", b: [1, 2]}`, nil, false},
		"imports nothing, has #do":    {`x: {#do: "encode", #provider: "base64"}`, nil, true},
		"imports a provider package":  {"import \"vela/base64\"\nx: 1", nil, true},
		"imports a provider, aliased": {"import b64 \"vela/base64\"\nx: 1", nil, true},
		"imports a CUE builtin only":  {"import \"strings\"\nout: strings.ToUpper(\"a\")", nil, false},
		"#do inside a comment":        {"// mentions #do\nout: 1", nil, true},
		"#do as a quoted label":       {`out: "#do": "encode"`, nil, true},
		"data as a Go map": {
			`parameter: {...}`, []CompileOption{WithData("parameter", map[string]any{"k": "v"})}, false,
		},
		"data as a cue.Value": {
			`parameter: {...}`,
			[]CompileOption{WithData("parameter", cuecontext.New().CompileString(`k: "v"`))},
			true,
		},
		"an intra-resolve mutation": {
			`out: 1`,
			[]CompileOption{WithIntraResolveMutation("x", func(_ context.Context, v cue.Value) (cue.Value, error) { return v, nil })},
			true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseFile("-", tt.src, parser.ParseComments)
			require.NoError(t, err)
			cfg := NewCompileConfig(tt.opts...)
			require.Equal(t, tt.want, c.mayContainCalls(tt.src, f, cfg, c.PackageManager.GetImports()))
		})
	}
}

// TestGoDataCannotMakeACall is the claim the skip rests on rather than a
// guess: a "#do" arriving as Go data is an ordinary field, not a definition,
// and the resolver does not see it.
func TestGoDataCannotMakeACall(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	val, err := c.CompileStringWithOptions(context.Background(), `parameter: {...}`,
		WithData("parameter", map[string]any{"#do": "encode", "#provider": "base64"}))
	require.NoError(t, err)

	node := val.LookupPath(cue.ParsePath("parameter"))
	require.True(t, node.Exists())
	require.False(t, node.LookupPath(cue.ParsePath("#do")).Exists(),
		`a filled "#do" must not be reachable as a definition, or skipping would lose a call`)

	bs, err := val.MarshalJSON()
	require.NoError(t, err)
	require.Contains(t, string(bs), `"#do"`, "it is still there, as an ordinary field")
}

// TestMayGrowArcs checks the test that decides whether the resolver may stop
// after running the calls a build produced, rather than searching the value
// again. Stopping wrongly means a call revealed later never runs, silently.
// See TestImportedComprehensionIsSeen for the other place one can hide.
func TestMayGrowArcs(t *testing.T) {
	for name, tt := range map[string]struct {
		src  string
		want bool
	}{
		"no comprehension":  {`a: 1, b: {c: "x"}`, false},
		"a for at the top":  {"a: {for i in [1,2] {\"\\(i)\": i}}", true},
		"a for nested deep": {"a: b: c: {for i in [1,2] {\"\\(i)\": i}}", true},
		"an if guard": {`a: 1
if a > 0 {b: 2}`, true},
		"an if inside a struct":      {`a: {x: 1, if x > 0 {y: 2}}`, true},
		"a for inside a list":        {"a: [for i in [1,2] {i}]", true},
		"a definition holding a for": {"#d: {for i in [1,2] {\"\\(i)\": i}}", true},
		"the word for in a string":   {`a: "for i in x"`, false},
		"a comment mentioning for":   {"// for i in x\na: 1", false},
		// a computed label makes no arc until what it names is concrete, so a
		// call under one is invisible until that call has run
		"an interpolated label":     {`b: {"\(a)": 1}`, true},
		"a dynamic label in parens": {`b: {(a): 1}`, true},
		"an interpolated value":     {`b: {k: "\(a)"}`, false},
		"a quoted label, no interp": {`b: {"k": 1}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseFile("-", tt.src, parser.ParseComments)
			require.NoError(t, err)
			require.Equal(t, tt.want, mayGrowArcs(f))
		})
	}
}

// TestImportedComprehensionIsSeen covers the half of the question the template
// cannot answer. A provider package's own CUE is built into the value too, and
// a definition there can wrap a call in a comprehension, so a template with
// none of its own still has to be searched again.
func TestImportedComprehensionIsSeen(t *testing.T) {
	plain, err := NewInternalPackageForTest("plain", `
package plain

#Echo: {#do: "echo", #provider: "plain", $params: string, $returns?: string}`)
	require.NoError(t, err)
	looping, err := NewInternalPackageForTest("looping", `
package looping

#Chain: {
	in:    string
	first: {#do: "echo", #provider: "looping", $params: in, $returns?: string}
	if first.$returns != _|_ {
		second: {#do: "echo", #provider: "looping", $params: first.$returns, $returns?: string}
	}
}`)
	require.NoError(t, err)

	src := `a: 1`
	f, err := parser.ParseFile("-", src, parser.ParseComments)
	require.NoError(t, err)

	quiet := NewCompilerWithInternalPackages(plain)
	require.False(t, quiet.mayRevealCalls(f, NewCompileConfig(), quiet.PackageManager.GetImports()),
		"a package with no comprehension leaves nothing to find")
	loud := NewCompilerWithInternalPackages(looping)
	require.True(t, loud.mayRevealCalls(f, NewCompileConfig(), loud.PackageManager.GetImports()),
		"a comprehension in an imported package must be seen")
}

// NewInternalPackageForTest builds a package with no provider functions, which
// is all mayRevealCalls needs to look at.
func NewInternalPackageForTest(name, template string) (cuexruntime.Package, error) {
	return cuexruntime.NewInternalPackage(name, template, map[string]cuexruntime.ProviderFn{})
}

// TestMutationAndValueDataForceASearch: neither a mutation nor a cue.Value
// being filled in can be read from here, so both mean the value has to be
// searched again. Deciding otherwise loses a call they revealed, silently.
func TestMutationAndValueDataForceASearch(t *testing.T) {
	plain, err := NewInternalPackageForTest("quiet", `
package quiet

#Echo: {#do: "echo", #provider: "quiet", $params: string, $returns?: string}`)
	require.NoError(t, err)
	c := NewCompilerWithInternalPackages(plain)
	f, err := parser.ParseFile("-", `a: 1`, parser.ParseComments)
	require.NoError(t, err)

	imports := c.PackageManager.GetImports()
	require.False(t, c.mayRevealCalls(f, NewCompileConfig(), imports),
		"nothing here can reveal a call")
	require.True(t, c.mayRevealCalls(f, NewCompileConfig(
		WithIntraResolveMutation("m", func(_ context.Context, v cue.Value) (cue.Value, error) {
			return v, nil
		})), imports), "a mutation replaces the value and cannot be read from here")
	require.True(t, c.mayRevealCalls(f, NewCompileConfig(
		WithData("d", cuecontext.New().CompileString(`x: 1`))), imports),
		"a cue.Value being filled in can carry anything")
	require.False(t, c.mayRevealCalls(f, NewCompileConfig(
		WithData("d", map[string]any{"x": 1})), imports),
		"a Go value fills ordinary fields only")
}
