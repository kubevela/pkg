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

package hash_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/hash"
)

type testStruct struct {
	Val int
}

func TestComputeHash(t *testing.T) {
	a := testStruct{Val: 1}
	b := &testStruct{Val: 1}
	c := &testStruct{Val: 2}
	ha, err := hash.ComputeHash(a)
	require.NoError(t, err)
	hb, err := hash.ComputeHash(b)
	require.NoError(t, err)
	hc, err := hash.ComputeHash(c)
	require.NoError(t, err)
	require.Equal(t, ha, hb)
	require.NotEqual(t, ha, hc)
}
