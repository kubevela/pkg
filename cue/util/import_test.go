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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/util"
)

func TestImport(t *testing.T) {
	// Test Normal
	bi, err := util.BuildImport("vela/test", map[string]string{
		"a": `#TestA: hello: "hello"`,
		"b": `#TestB: hello: "hello"`,
	})
	require.NoError(t, err)
	require.Equal(t, "test", bi.PkgName)

	// Test invalid CUE
	bi, err = util.BuildImport("vela/test-bad", map[string]string{"-": `bad-val!@#`})
	require.Error(t, err)
}
