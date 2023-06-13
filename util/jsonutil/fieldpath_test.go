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
	"k8s.io/utils/pointer"

	"github.com/kubevela/pkg/util/jsonutil"
)

func TestParseFields(t *testing.T) {
	testcases := map[string]struct {
		Input  string
		Fields []jsonutil.Field
		Error  string
	}{
		"normal": {
			Input: `a."b\s".20."30".f`,
			Fields: []jsonutil.Field{
				{Label: "a"},
				{Label: `b\s`},
				{Label: "20", Index: pointer.Int64(20)},
				{Label: "30"},
				{Label: "f"},
			},
		},
		"unexpected char after quote": {
			Input: `"a"+`,
			Error: "unexpected char",
		},
		"unexpected period": {
			Input: `a+.`,
			Error: "unexpected end period",
		},
		"unexpected slash": {
			Input: `a\`,
			Error: "unexpected slash",
		},
		"empty field": {
			Input: "a..b",
			Error: "empty field",
		},
	}
	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			fields, err := jsonutil.ParseFields(tt.Input)
			if len(tt.Error) > 0 {
				require.ErrorContains(t, err, tt.Error)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Fields, fields)
			}
		})
	}
}

func TestLookupPath(t *testing.T) {
	testcases := map[string]struct {
		Input string
		Path  string
		Value any
		Error bool
	}{
		"normal": {
			Input: `{"a": {"b": [{"20": 1}, {"20": 2.0}]}}`,
			Path:  `a.b.1."20"`,
			Value: 2.0,
		},
		"return-struct": {
			Input: `{"a": {"b": [{"20": 1}, {"20": {"x": "y"}}]}}`,
			Path:  `a.b.1."20"`,
			Value: map[string]interface{}{"x": "y"},
		},
		"empty": {
			Input: `{"a": {"b": [{"20": 1}, {"20": 2}]}}`,
			Path:  "d.e",
			Value: nil,
		},
		"path error": {
			Input: `{"a": {"b": [{"20": 1}, {"20": 2}]}}`,
			Path:  `"badPath"+`,
			Error: true,
		},
		"type mismatch": {
			Input: `{"a": {"b": [{"20": 1}, {"20": 2}]}}`,
			Path:  `a.2`,
			Value: nil,
		},
		"type mismatch val": {
			Input: `{"a": {"b": [{"20": 1}, {"20": 2}]}}`,
			Path:  `a.b.0."20".y`,
			Value: nil,
		},
		"out of range": {
			Input: `{"arr": [0,1]}`,
			Path:  "arr.3",
			Value: nil,
		},
	}
	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			obj := map[string]interface{}{}
			require.NoError(t, json.Unmarshal([]byte(tt.Input), &obj))
			val, err := jsonutil.LookupPath(obj, tt.Path)
			if tt.Error {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.Value, val)
			}
		})
	}
}
