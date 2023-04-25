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

package runtime_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/runtime"
)

type testStruct struct{}

func TestIsNil(t *testing.T) {
	for name, tt := range map[string]struct {
		Input  any
		Output bool
	}{
		"string":     {Input: "", Output: true},
		"nil":        {Input: nil, Output: true},
		"nil-ptr":    {Input: (*testStruct)(nil), Output: true},
		"struct-ptr": {Input: &testStruct{}, Output: false},
		"struct-val": {Input: testStruct{}, Output: false},
		"nil-arr":    {Input: []string(nil), Output: true},
		"empty-arr":  {Input: []string{}, Output: false},
		"nil-map":    {Input: map[string]any(nil), Output: true},
		"empty-map":  {Input: map[string]any{}, Output: false},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.Output, runtime.IsNil(tt.Input))
		})
	}
}
