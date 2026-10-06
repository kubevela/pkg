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
	"math"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cel/template"
)

func TestElementsCompatibleWidensOnlyIntoDouble(t *testing.T) {
	cc := cuecontext.New()
	ints := cc.CompileString(`[...int]`)
	doubles := cc.CompileString(`[...float]`)
	ok, _, _ := ElementsCompatible(cel.ListType(cel.IntType), doubles)
	require.True(t, ok, "an int is a valid double")
	ok, _, _ = ElementsCompatible(cel.ListType(cel.DoubleType), ints)
	require.False(t, ok, "a double may carry a fraction an int cannot")
}

func TestElementsCompatibleComparesMapKeys(t *testing.T) {
	target := cuecontext.New().CompileString(`[string]: string`)
	ok, _, _ := ElementsCompatible(cel.MapType(cel.IntType, cel.StringType), target)
	require.False(t, ok)
	ok, _, _ = ElementsCompatible(cel.MapType(cel.StringType, cel.StringType), target)
	require.True(t, ok)
}

func TestLargeFloatsAreNotTruncatedToInts(t *testing.T) {
	e := testEngine(t)
	got, err := e.Eval(e.DynEnv(), `alpha.n`, map[string]interface{}{"alpha": map[string]interface{}{"n": 1e20}})
	require.NoError(t, err)
	require.Equal(t, 1e20, got)
	got, err = e.Eval(e.DynEnv(), `alpha.n + 1`, map[string]interface{}{"alpha": map[string]interface{}{"n": float64(41)}})
	require.NoError(t, err)
	require.Equal(t, int64(42), got)
}

func TestEveryGoIntegerWidthIsUsable(t *testing.T) {
	e := testEngine(t)
	for _, n := range []interface{}{int(1), int8(1), int16(1), int32(1), int64(1), uint(1), uint8(1), uint16(1), uint32(1), uint64(1)} {
		got, err := e.Eval(e.DynEnv(), `alpha.n + 1`, map[string]interface{}{"alpha": map[string]interface{}{"n": n}})
		require.NoError(t, err, "%T", n)
		require.Equal(t, int64(2), got, "%T", n)
	}
	got, err := e.Eval(e.DynEnv(), `alpha.n`, map[string]interface{}{"alpha": map[string]interface{}{"n": uint64(math.MaxUint64)}})
	require.NoError(t, err)
	require.Equal(t, uint64(math.MaxUint64), got, "too large for an int, so kept unsigned")
}

// Text can embed a single value only. A list or map has no one spelling as text,
// so it is refused instead of rendered in Go syntax.
func TestACollectionCannotBeEmbeddedInText(t *testing.T) {
	e := testEngine(t)
	in := map[string]interface{}{"alpha": map[string]interface{}{"tags": []interface{}{"a", "b"}, "m": map[string]interface{}{"k": "v"}}}
	_, err := e.EvalProperty(e.DynEnv(), `--tags=$(alpha.tags)`, in)
	require.ErrorContains(t, err, "is a list")
	_, err = e.EvalProperty(e.DynEnv(), `m=$(alpha.m)`, in)
	require.ErrorContains(t, err, "is a map")
	got, err := e.EvalProperty(e.DynEnv(), `--tags=$(alpha.tags.join(","))`, in)
	require.NoError(t, err)
	require.Equal(t, "--tags=a,b", got)
	got, err = e.EvalProperty(e.DynEnv(), `$(alpha.tags)`, in)
	require.NoError(t, err, "a whole value keeps its type")
	require.Equal(t, []interface{}{"a", "b"}, got)
}

func TestMapLeavesNilContainersNil(t *testing.T) {
	out, err := template.Map(map[string]interface{}{"m": map[string]interface{}(nil), "l": []interface{}(nil)}, "",
		func(_, raw string) (interface{}, error) { return raw, nil })
	require.NoError(t, err)
	m := out.(map[string]interface{})
	require.Nil(t, m["m"])
	require.Nil(t, m["l"])
}
