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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/maps"
	"github.com/kubevela/pkg/util/slices"
)

func TestSyncMap(t *testing.T) {
	_m := map[string]int{
		"a": 1,
		"b": 2,
	}
	m := maps.NewSyncMapFrom(_m)
	v, ok := m.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, v)
	m.Set("c", 3)
	m.Del("a")
	require.True(t, slices.Contains(m.Keys(), "b"))
	require.True(t, slices.Contains(m.Keys(), "c"))
	require.True(t, slices.Contains(m.Values(), 2))
	require.True(t, slices.Contains(m.Values(), 3))
	cnt := 0
	m.Range(func(i string, v int) { cnt += v })
	require.Equal(t, 5, cnt)
	m.Load(map[string]int{"c": 3})
	require.Equal(t, 1, len(m.Keys()))
}
