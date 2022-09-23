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

package k8s_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/pkg/util/k8s"
)

func TestGetKindForObject(t *testing.T) {
	testcases := map[string]struct {
		obj      runtime.Object
		trimList bool
		expected string
	}{
		"unstructured": {
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind": "ExampleList",
			}},
			expected: "ExampleList",
		},
		"structured": {
			obj:      &corev1.ConfigMapList{},
			expected: "ConfigMapList",
		},
		"structured-list": {
			obj:      &corev1.ConfigMapList{},
			trimList: true,
			expected: "ConfigMap",
		},
	}
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t,
				testcase.expected,
				k8s.GetKindForObject(testcase.obj, testcase.trimList))
		})
	}
}

func TestIsUnstructuredObject(t *testing.T) {
	testcases := map[string]struct {
		obj      runtime.Object
		expected bool
	}{
		"unstructured": {
			obj:      &unstructured.Unstructured{},
			expected: true,
		},
		"unstructured-list": {
			obj:      &unstructured.UnstructuredList{},
			expected: true,
		},
		"structured": {
			obj:      &corev1.ConfigMapList{},
			expected: false,
		},
	}
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t,
				testcase.expected,
				k8s.IsUnstructuredObject(testcase.obj))
		})
	}
}

type testNoMetaObject struct {
	runtime.Object
}

func TestAddAnnotation(t *testing.T) {
	testcases := map[string]struct {
		obj     runtime.Object
		key     string
		value   string
		wantErr string
	}{
		"unstructured": {
			obj:   &unstructured.Unstructured{},
			key:   "test-key",
			value: "test-value",
		},
		"unstructured-update": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"test-key": "test-value",
						},
					},
				},
			},
			key:   "test-key",
			value: "test-value-new",
		},
		"no-meta": {
			obj:     &testNoMetaObject{},
			wantErr: "object does not implement the Object interfaces",
		},
	}
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			err := k8s.AddAnnotation(testcase.obj, testcase.key, testcase.value)
			if testcase.wantErr != "" {
				r.Equal(err.Error(), testcase.wantErr)
				return
			}
			r.NoError(err)
			result := k8s.GetAnnotation(testcase.obj, testcase.key)
			r.Equal(testcase.value, result)
		})
	}
}

func TestGetAnnotation(t *testing.T) {
	testcases := map[string]struct {
		obj      runtime.Object
		key      string
		expected string
		wantErr  string
	}{
		"unstructured": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"test-key": "test-value",
						},
					},
				},
			},
			key:      "test-key",
			expected: "test-value",
		},
		"unstructured-empty": {
			obj:      &unstructured.Unstructured{},
			key:      "test-key",
			expected: "",
		},
		"no-meta": {
			obj:     &testNoMetaObject{},
			wantErr: "object does not implement the Object interfaces",
		},
	}
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			result := k8s.GetAnnotation(testcase.obj, testcase.key)
			r.Equal(testcase.expected, result)
		})
	}
}

func TestDeleteAnnotation(t *testing.T) {
	testcases := map[string]struct {
		obj     runtime.Object
		key     string
		wantErr string
	}{
		"unstructured": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"test-key": "test-value",
						},
					},
				},
			},
			key: "test-key",
		},
		"unstructured-empty": {
			obj: &unstructured.Unstructured{},
			key: "test-key",
		},
		"no-meta": {
			obj:     &testNoMetaObject{},
			wantErr: "object does not implement the Object interfaces",
		},
	}
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			err := k8s.DeleteAnnotation(testcase.obj, testcase.key)
			if testcase.wantErr != "" {
				r.Equal(err.Error(), testcase.wantErr)
				return
			}
			r.NoError(err)
			result := k8s.GetAnnotation(testcase.obj, testcase.key)
			r.Equal("", result)
		})
	}
}

func TestAddLabel(t *testing.T) {
	testcases := map[string]struct {
		obj     runtime.Object
		key     string
		value   string
		wantErr string
	}{
		"unstructured": {
			obj:   &unstructured.Unstructured{},
			key:   "test-key",
			value: "test-value",
		},
		"unstructured-update": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"test-key": "test-value",
						},
					},
				},
			},
			key:   "test-key",
			value: "test-value-new",
		},
		"no-meta": {
			obj:     &testNoMetaObject{},
			wantErr: "object does not implement the Object interfaces",
		},
	}
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			err := k8s.AddLabel(testcase.obj, testcase.key, testcase.value)
			if testcase.wantErr != "" {
				r.Equal(err.Error(), testcase.wantErr)
				return
			}
			r.NoError(err)
			result := k8s.GetLabel(testcase.obj, testcase.key)
			r.Equal(testcase.value, result)
		})
	}
}

func TestGetLabel(t *testing.T) {
	testcases := map[string]struct {
		obj      runtime.Object
		key      string
		expected string
		wantErr  string
	}{
		"unstructured": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"test-key": "test-value",
						},
					},
				},
			},
			key:      "test-key",
			expected: "test-value",
		},
		"unstructured-empty": {
			obj:      &unstructured.Unstructured{},
			key:      "test-key",
			expected: "",
		},
		"no-meta": {
			obj:     &testNoMetaObject{},
			wantErr: "object does not implement the Object interfaces",
		},
	}
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			result := k8s.GetLabel(testcase.obj, testcase.key)
			r.Equal(testcase.expected, result)
		})
	}
}

func TestDeleteLabel(t *testing.T) {
	testcases := map[string]struct {
		obj     runtime.Object
		key     string
		wantErr string
	}{
		"unstructured": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"test-key": "test-value",
						},
					},
				},
			},
			key: "test-key",
		},
		"unstructured-empty": {
			obj: &unstructured.Unstructured{},
			key: "test-key",
		},
		"no-meta": {
			obj:     &testNoMetaObject{},
			wantErr: "object does not implement the Object interfaces",
		},
	}
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			err := k8s.DeleteLabel(testcase.obj, testcase.key)
			if testcase.wantErr != "" {
				r.Equal(err.Error(), testcase.wantErr)
				return
			}
			r.NoError(err)
			result := k8s.GetLabel(testcase.obj, testcase.key)
			r.Equal("", result)
		})
	}
}
