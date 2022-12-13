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

package sync_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/sync"
)

func TestSyncMap(t *testing.T) {
	m := sync.NewMap[string, int]()
	m.Set("1", 1)
	val, found := m.Get("1")
	require.True(t, found)
	require.Equal(t, 1, val)
	require.Equal(t, map[string]int{"1": 1}, m.Data())
	m.Del("1")
	_, found = m.Get("1")
	require.False(t, found)
}
