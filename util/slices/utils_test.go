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

package slices_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/pointer"

	"github.com/kubevela/pkg/util/slices"
)

func TestMap(t *testing.T) {
	arr := slices.Map([]int{1, 2, 3}, func(i int) string {
		return fmt.Sprintf("val:%d", i)
	})
	require.Equal(t, []string{"val:1", "val:2", "val:3"}, arr)
}

func TestFilter(t *testing.T) {
	arr := slices.Filter([]int{1, 2, 3}, func(i int) bool {
		return i%2 == 1
	})
	require.Equal(t, []int{1, 3}, arr)
}

func TestIndex(t *testing.T) {
	idx := slices.Index([]int{1, 2, 3}, func(i int) bool {
		return i%2 == 0
	})
	require.Equal(t, 1, idx)
	idx = slices.Index([]int{1, 2, 3}, func(i int) bool {
		return i%4 == 0
	})
	require.Equal(t, -1, idx)
}

func TestFind(t *testing.T) {
	val := slices.Find([]int{1, 2, 3}, func(i int) bool {
		return i%2 == 0
	})
	require.Equal(t, pointer.Int(2), val)
	val = slices.Find([]int{1, 2, 3}, func(i int) bool {
		return i%4 == 0
	})
	require.Nil(t, val)
}

func TestFlatten(t *testing.T) {
	arr := slices.Flatten([][]int{{1, 2, 3}, {2, 4, 6}})
	require.Equal(t, []int{1, 2, 3, 2, 4, 6}, arr)
}

func TestAll(t *testing.T) {
	require.False(t, slices.All([]int{1, 2, 3}, func(i int) bool { return i%2 == 0 }))
	require.True(t, slices.All([]int{0, 2, 4}, func(i int) bool { return i%2 == 0 }))
}

func TestAny(t *testing.T) {
	require.True(t, slices.Any([]int{1, 2, 3}, func(i int) bool { return i%2 == 0 }))
	require.False(t, slices.Any([]int{1, 3, 5}, func(i int) bool { return i%2 == 0 }))
}

func TestCount(t *testing.T) {
	require.Equal(t, 2, slices.Count([]int{1, 2, 3}, func(i int) bool { return i%2 != 0 }))
}
