/*
Copyright 2023 The KubeVela Authors.

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

package util_test

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/util"
)

func TestIterate(t *testing.T) {
	value := cuecontext.New().CompileString(`
	a: ["a", "b", "c"]
	#x: string
	b: {
		c: "val"
		d: "d"
	}`)
	var results []string
	stop := util.Iterate(value, func(v cue.Value) (stop bool) {
		if s, err := v.String(); err == nil {
			results = append(results, s)
			if s == "val" {
				return true
			}
		}
		return false
	})
	require.Equal(t, []string{"a", "b", "c", "val"}, results)
	require.True(t, stop)
}

func TestIterateWithOrder(t *testing.T) {
	value := cuecontext.New().CompileString(`
		a: "a" @step(2)
		#x: string
		x: "x"
		b: {
			c: "val" @step(1)
			d: "d" @step(2)
			e: {
				f: "f"
				g: "g"
			}
		} @step(1)
	`)
	var results []string
	stop := util.Iterate(value, func(v cue.Value) (stop bool) {
		results = append(results, v.Path().String())
		return false
	})
	require.Equal(t, []string{"b.c", "b.d", "b.e.f", "b.e.g", "b.e", "b", "a", "x", ""}, results)
	require.False(t, stop)
}

func TestIterateWithIfComprehension(t *testing.T) {
	value := cuecontext.New().CompileString(`
		step1: {} @step(1)
		step2: {prefix: step1.value} @step(2)
		if step2.value > 100 {
			step2_3: {prefix: step2.value} @step(3)
		}
		step3: {prefix: step2.value} @step(4)
		step4: {prefix: step3.value} @step(5)
	`)
	newValue := value
	executed := map[string]bool{}
	for cnt := 100; cnt < 1000; cnt++ {
		var next *cue.Value
		util.Iterate(newValue, func(v cue.Value) (stop bool) {
			path := v.Path().String()
			_, done := executed[path]
			if !done && strings.Contains(path, "step") && !strings.Contains(path, ".") {
				next = &v
				return true
			}
			return false
		})
		if next == nil {
			break
		}
		newValue = newValue.FillPath(next.Path(), next.FillPath(cue.ParsePath("value"), cnt))
		executed[next.Path().String()] = true
	}
	for k, v := range map[string]int64{
		"step1.value":   100,
		"step2.value":   101,
		"step2_3.value": 102,
		"step3.value":   103,
		"step4.value":   104,
	} {
		_v, err := newValue.LookupPath(cue.ParsePath(k)).Int64()
		require.NoError(t, err)
		require.Equal(t, v, _v)
	}
}
