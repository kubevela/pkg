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
	"errors"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// paramTarget is a parameter block as a Target, looked up by property name.
func paramTarget(t *testing.T, text string) Target {
	t.Helper()
	v := cuecontext.New().CompileString(text)
	require.NoError(t, v.Err())
	return TargetFunc(func(property string) (cue.Value, bool, bool) {
		iter, err := v.Fields(cue.Optional(true))
		require.NoError(t, err)
		for iter.Next() {
			if iter.Selector().Unquoted() != property {
				continue
			}
			_, defaulted := iter.Value().Default()
			return iter.Value(), !iter.IsOptional() && !defaulted, true
		}
		return cue.Value{}, false, false
	})
}

func checkTypes(t *testing.T, props map[string]interface{}) map[string]error {
	t.Helper()
	env, err := typedEnvText(map[string]string{
		"cfg": `{host: string, port: int, ratio: float, tags: [...string], note?: string}`,
	})
	require.NoError(t, err)
	p, err := fixture.Plan(props)
	require.NoError(t, err)
	target := paramTarget(t, `{port: int, replicas: int, ids: [...int], name: string, label: string, opt?: string, n: int}`)
	faults := p.CheckTypes(env, target, TypeOptions{Optional: func(r Read) bool {
		return r.Root == "source" && len(r.Path) == 2 && r.Path[1] == "note"
	}})
	out := map[string]error{}
	for _, f := range faults {
		out[f.Property] = f.Err
	}
	return out
}

func TestCheckTypesAssertsEachResultFitsItsTarget(t *testing.T) {
	got := checkTypes(t, map[string]interface{}{
		"port":     "$(source.cfg.port)",
		"replicas": "$(source.cfg.ratio)",
		"ids":      "$(source.cfg.tags)",
		"name":     "x-$(source.cfg.tags)",
		"label":    "$(source.cfg.note)",
		"opt":      "$(source.cfg.note)",
		"n":        "$(component.db.output.count)",
		"extra":    "$(source.cfg.host)",
	})

	var m *TypeMismatch
	require.True(t, errors.As(got["replicas"], &m), "a double may carry a fraction an int cannot: %v", got["replicas"])
	require.Equal(t, "number", m.Got)
	require.Equal(t, "int", m.Want)

	require.True(t, errors.As(got["ids"], &m), "%v", got["ids"])
	require.True(t, m.Elements)
	require.Equal(t, "list(int)", m.Want)

	require.ErrorContains(t, got["name"], "cannot be combined with text")

	var absent *MayBeAbsent
	require.True(t, errors.As(got["label"], &absent), "an unguarded optional read feeds a required target: %v", got["label"])
	require.Equal(t, "source.cfg.note", absent.Read.String())

	for _, fine := range []string{"port", "opt", "n", "extra"} {
		require.NoError(t, got[fine], fine)
	}
}

func TestCheckTypesRefusesAWrongKind(t *testing.T) {
	got := checkTypes(t, map[string]interface{}{"port": "$(source.cfg.host)"})
	var m *TypeMismatch
	require.True(t, errors.As(got["port"], &m), "%v", got["port"])
	require.Equal(t, "string", m.Got)
	require.False(t, m.Elements)
}

func TestCheckTypesReportsACompileErrorAgainstTheTypedEnv(t *testing.T) {
	got := checkTypes(t, map[string]interface{}{"port": "$(source.cfg.nope)"})
	require.ErrorContains(t, got["port"], "undefined field 'nope'")
}

func TestCheckTypesAcceptsAGuardedOptionalRead(t *testing.T) {
	got := checkTypes(t, map[string]interface{}{"label": `$(has(source.cfg.note) ? source.cfg.note : "none")`})
	require.NoError(t, got["label"])
}

// With no target, only what the typed environment itself refuses is reported.
func TestCheckTypesWithoutATarget(t *testing.T) {
	env, err := typedEnvText(map[string]string{"cfg": `{host: string}`})
	require.NoError(t, err)
	p, err := fixture.Plan(map[string]interface{}{"a": "$(source.cfg.host)", "b": "$(source.cfg.nope)"})
	require.NoError(t, err)
	faults := p.CheckTypes(env, nil, TypeOptions{})
	require.Len(t, faults, 1)
	require.Equal(t, "b", faults[0].Property)
}

// CEL types CUE's number as double, which would refuse a number that holds an
// integer. A bare read is judged by the kind its schema declares; a computed
// result keeps CEL's type.
func TestCheckTypesJudgesABareReadByItsDeclaredKind(t *testing.T) {
	env, err := typedEnvText(map[string]string{"cfg": `{n: number, ratio: float, ns: [...number]}`})
	require.NoError(t, err)
	p, err := fixture.Plan(map[string]interface{}{
		"a": "$(source.cfg.n)",
		"b": "$(source.cfg.ratio)",
		"c": "$(source.cfg.n * 2.0)",
		"d": "$(source.cfg.ns[0])",
	})
	require.NoError(t, err)
	declared := map[string]cue.Kind{"n": cue.NumberKind, "ratio": cue.FloatKind, "0": cue.NumberKind}
	target := TargetFunc(func(string) (cue.Value, bool, bool) {
		return cuecontext.New().CompileString("int"), true, true
	})
	faults := p.CheckTypes(env, target, TypeOptions{DeclaredKind: func(r Read) (cue.Kind, bool) {
		k, ok := declared[r.Path[len(r.Path)-1]]
		return k, ok
	}})
	got := map[string]bool{}
	for _, f := range faults {
		got[f.Property] = true
	}
	require.Equal(t, map[string]bool{"b": true, "c": true}, got, "a number fits an int; a float or a double result does not")
}

func TestKindFits(t *testing.T) {
	require.True(t, KindFits(cue.IntKind, cue.NumberKind))
	require.True(t, KindFits(cue.NumberKind, cue.IntKind), "a number may hold an integer")
	require.False(t, KindFits(cue.FloatKind, cue.IntKind))
	require.True(t, KindFits(cue.TopKind, cue.StringKind))
	require.False(t, KindFits(cue.StringKind, cue.IntKind))
}
