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

package jsonutil_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/jsonutil"
)

func TestDropField(t *testing.T) {
	testCases := map[string]struct {
		Input  string
		Fields []string
		Output string
	}{
		"empty": {
			Input:  `{"a":1}`,
			Fields: nil,
			Output: `{"a":1}`,
		},
		"not-found": {
			Input:  `{"a":1}`,
			Fields: []string{"b"},
			Output: `{"a":1}`,
		},
		"type-not-match": {
			Input:  `{"a":[1]}`,
			Fields: []string{"a", "b", "c"},
			Output: `{"a":[1]}`,
		},
		"key-not-found": {
			Input:  `{"a":{"b":3}}`,
			Fields: []string{"b", "a", "b"},
			Output: `{"a":{"b":3}}`,
		},
		"nil": {
			Input:  `{"a":null}`,
			Fields: []string{"a", "b"},
			Output: `{"a":null}`,
		},
		"nested-drop": {
			Input:  `{"a":{"b":3}}`,
			Fields: []string{"a", "b"},
			Output: `{"a":{}}`,
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			m1, m2 := map[string]any{}, map[string]any{}
			require.NoError(t, json.Unmarshal([]byte(tt.Input), &m1))
			require.NoError(t, json.Unmarshal([]byte(tt.Output), &m2))
			jsonutil.DropField(m1, tt.Fields...)
			require.Equal(t, m2, m1)
		})
	}
}
