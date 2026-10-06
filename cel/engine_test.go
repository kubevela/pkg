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

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/require"
	apiservercel "k8s.io/apiserver/pkg/cel"

	"github.com/kubevela/pkg/cel/template"
)

// testEngine has two plain roots and one with qualifiers, under names no host
// uses, so nothing here passes by matching a host's roots.
func testEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := NewEngine(
		Root{Name: "alpha"},
		Root{Name: "beta"},
		Root{Name: "peer", QualifiedAt: 1, Qualifiers: []Qualifier{
			{Name: "at"},
			{Name: "everywhere", Result: cel.ListType(cel.DynType)},
		}},
	)
	require.NoError(t, err)
	return e
}

func TestEngineDeclaresOnlyItsRoots(t *testing.T) {
	e := testEngine(t)
	_, err := e.OutputType(e.DynEnv(), `alpha.x + beta.y`)
	require.NoError(t, err)
	_, err = e.OutputType(e.DynEnv(), `source.x`)
	require.ErrorContains(t, err, "undeclared reference to 'source'")
}

func TestNewEngineRefusesBadRoots(t *testing.T) {
	for name, roots := range map[string][]Root{
		"empty name":     {{Name: ""}},
		"not an ident":   {{Name: "my-root"}},
		"duplicate root": {{Name: "a"}, {Name: "a"}},
		"one qualifier, two result types": {
			{Name: "a", Qualifiers: []Qualifier{{Name: "at"}}},
			{Name: "b", Qualifiers: []Qualifier{{Name: "at", Result: cel.ListType(cel.DynType)}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewEngine(roots...)
			require.Error(t, err)
		})
	}
}

func TestReferencesKeepOnlyRegisteredRoots(t *testing.T) {
	e := testEngine(t)
	refs, err := e.PropertyReferences(`[1].map(x, x + alpha.n)[0] + size(beta.list)`)
	require.NoError(t, err)
	require.Equal(t, []template.Reference{
		{Root: "alpha", Path: []string{"n"}},
		{Root: "beta", Path: []string{"list"}},
	}, refs)
}

func TestQualifiersBecomePathSegments(t *testing.T) {
	e := testEngine(t)
	refs, err := e.PropertyReferences(`peer.db.at("east").status.host`)
	require.NoError(t, err)
	require.Equal(t, []template.Reference{{Root: "peer", Path: []string{
		"db", template.QualifierSegment(template.Call("at", "east")), "status", "host",
	}}}, refs)
	require.Equal(t, `peer.db.at("east").status.host`, refs[0].String())
}

func TestQualifiersLookUpWhatWasDelivered(t *testing.T) {
	e := testEngine(t)
	in := map[string]interface{}{"peer": map[string]interface{}{
		"db": map[string]interface{}{QualifiedKey: map[string]interface{}{
			template.Call("at", "east"):        map[string]interface{}{"host": "db.east"},
			template.Call("everywhere", "all"): []interface{}{"x", "y"},
		}},
	}}

	got, err := e.Eval(e.DynEnv(), `peer.db.at("east").host`, in)
	require.NoError(t, err)
	require.Equal(t, "db.east", got)

	got, err = e.Eval(e.DynEnv(), `size(peer.db.everywhere("all"))`, in)
	require.NoError(t, err)
	require.Equal(t, int64(2), got)

	_, err = e.Eval(e.DynEnv(), `peer.db.at("west").host`, in)
	require.ErrorContains(t, err, `.at("west") was not delivered`)
}

func TestTypedValuesAreNotGuessed(t *testing.T) {
	e := testEngine(t)
	// JSON decodes every number as float64. Unmarked, an integral one is read
	// as an int; marked typed, a float stays a float.
	in := map[string]interface{}{
		"alpha": map[string]interface{}{"n": float64(2)},
		"beta":  Typed(map[string]interface{}{"ratio": float64(2), "count": 3}),
	}
	got, err := e.Eval(e.DynEnv(), `alpha.n + 1`, in)
	require.NoError(t, err)
	require.Equal(t, int64(3), got)

	got, err = e.Eval(e.DynEnv(), `beta.ratio * 2.0`, in)
	require.NoError(t, err)
	require.Equal(t, 4.0, got)

	got, err = e.Eval(e.DynEnv(), `beta.count + 1`, in)
	require.NoError(t, err, "a Go int is widened to CEL's int64 even when typed")
	require.Equal(t, int64(4), got)
}

func TestTypedEnvDeclaresTheRootsGiven(t *testing.T) {
	e := testEngine(t)
	decls := func() (map[string]*apiservercel.DeclType, error) {
		return map[string]*apiservercel.DeclType{
			"alpha": apiservercel.NewObjectType("test.alpha", map[string]*apiservercel.DeclField{
				"port": apiservercel.NewDeclField("port", apiservercel.StringType, true, nil, nil),
			}),
		}, nil
	}
	env, err := e.TypedEnv("k", decls)
	require.NoError(t, err)

	_, err = e.OutputType(env, `alpha.port + 1`)
	require.Error(t, err, "port is declared a string")
	out, err := e.OutputType(env, `beta.anything`)
	require.NoError(t, err, "a root given no decl stays an open map")
	require.Equal(t, cel.DynType, out)

	again, err := e.TypedEnv("k", func() (map[string]*apiservercel.DeclType, error) {
		t.Fatal("a cached key must not rebuild")
		return nil, nil
	})
	require.NoError(t, err)
	require.Same(t, env, again)
}

// Programs are cached by expression text for the permissive env alone. Two
// engines must not share that cache: the same text can compile under one set of
// roots and be undeclared under another.
func TestEnginesDoNotShareCompilations(t *testing.T) {
	withAlpha := testEngine(t)
	without, err := NewEngine(Root{Name: "beta"})
	require.NoError(t, err)

	_, err = withAlpha.Eval(withAlpha.DynEnv(), `alpha.x`, map[string]interface{}{"alpha": map[string]interface{}{"x": 1}})
	require.NoError(t, err)
	_, err = without.Eval(without.DynEnv(), `alpha.x`, map[string]interface{}{})
	require.ErrorContains(t, err, "undeclared reference to 'alpha'")
}

func TestCheckRefusesRootsOutsideTheSurface(t *testing.T) {
	e := testEngine(t)
	props := map[string]interface{}{"a": "$(alpha.x)", "b": []interface{}{"x-$(beta.y)"}}
	require.NoError(t, validateTree(e, props, "alpha", "beta"))
	require.ErrorContains(t, validateTree(e, props, "alpha"), `"beta" cannot be read here`)
}

// A qualifier goes only on a read of a root that declares it: anywhere else it
// could only fail when evaluated. A NUL marks a qualifier inside a read path, so
// no key may contain one.
func TestQualifiersGoOnlyOnTheirRoot(t *testing.T) {
	e := testEngine(t)
	for _, expr := range []string{
		`alpha.at("east").host`,
		`alpha.cfg.at("east")`,
		`"x".everywhere("all")`,
	} {
		_, err := e.PropertyReferences(expr)
		require.ErrorContains(t, err, "goes only on a peer read", expr)
	}
	_, err := e.PropertyReferences("peer.db.output[\"a\\u0000b\"]")
	require.ErrorContains(t, err, "NUL")

	refs, err := e.PropertyReferences(`peer.db.at("east").host`)
	require.NoError(t, err)
	require.Len(t, refs, 1)
}

// A qualifier named like a function or macro every environment offers would
// take that name over wherever it is called, so it is refused.
func TestNewEngineRefusesQualifierNamesTaken(t *testing.T) {
	for _, name := range []string{"contains", "split", "size", "map", "has"} {
		_, err := NewEngine(Root{Name: "peer", Qualifiers: []Qualifier{{Name: name}}})
		require.ErrorContains(t, err, "already", name)
	}
}

func TestNewEngineRefusesAQualifierDeclaredTwiceOnARoot(t *testing.T) {
	_, err := NewEngine(Root{Name: "peer", Qualifiers: []Qualifier{{Name: "at"}, {Name: "at"}}})
	require.ErrorContains(t, err, "twice")
}

// A qualifier selects among values delivered for one entry of its root, so it
// goes at the root's qualifier depth, straight after the entry's name, or after
// another qualifier; anywhere else it would find nothing delivered.
func TestQualifiersGoAtTheirDepth(t *testing.T) {
	e := testEngine(t)
	for _, expr := range []string{
		`peer.db.host.at("east")`,
		`peer.db.everywhere("all")[0].at("east")`,
		`peer[alpha.name].at("east").host`,
		`peer.at("east")`,
	} {
		_, err := e.PropertyReferences(expr)
		require.ErrorContains(t, err, "at and everywhere go straight after peer.<name>", expr)
	}
	for _, expr := range []string{
		`peer["db"].at("east").host`,
		`peer.db.at("east").at("west").host`,
	} {
		_, err := e.PropertyReferences(expr)
		require.NoError(t, err, expr)
	}
	_, err := e.PropertyReferences("peer.db.at(\"a\\u0000b\").host")
	require.ErrorContains(t, err, "NUL")
}
