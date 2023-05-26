/*
Copyright 2022 The KubeVela Authors.

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

package maps_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/maps"
	"github.com/kubevela/pkg/util/slices"
)

func TestUtils(t *testing.T) {
	m := map[string]int{
		"a": 1,
		"b": 2,
	}
	require.True(t, slices.Contains(maps.Keys(m), "a"))
	require.True(t, slices.Contains(maps.Keys(m), "b"))
	require.True(t, slices.Contains(maps.Values(m), 1))
	require.True(t, slices.Contains(maps.Values(m), 2))

	require.Equal(t, map[string]string{
		"a": "1",
		"b": "2",
	}, maps.Map(m, func(x int) string { return fmt.Sprintf("%d", x) }))

	_m := maps.Copy(m)
	require.Equal(t, 2, len(_m))

	_m = maps.Filter(m, func(k string, _ int) bool { return k == "a" })
	_m = maps.Map(_m, func(v int) int { return v + 1 })
	require.Equal(t, map[string]int{"a": 2}, _m)

	require.Equal(t, map[int]string{1: "0", 2: "1"}, maps.From([]int{0, 1}, func(i int) (int, string) {
		return i + 1, fmt.Sprintf("%d", i)
	}))
}
