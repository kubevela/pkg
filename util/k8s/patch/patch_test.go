/*
Copyright 2021 The KubeVela Authors.

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

package patch_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/pkg/util/k8s/patch"
	"github.com/kubevela/pkg/util/test/object"
)

func TestThreeWayMerge(t *testing.T) {
	cases := map[string]struct {
		current     runtime.Object
		modified    runtime.Object
		PatchAction *patch.PatchAction
		wantErr     string
		result      string
	}{
		"custom resources": {
			PatchAction: &patch.PatchAction{
				AnnoLastAppliedConfig: "last-applied/config",
				AnnoLastAppliedTime:   "last-applied/time",
				UpdateAnno:            true,
			},
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "default",
						"annotations": map[string]interface{}{
							"last-applied/config": "{\"kind\":\"Foo\",\"metadata\":{\"name\":\"test\",\"namespace\":\"default\"},\"data\":{\"k3\":\"v3\"}}",
						},
					},
					"data": map[string]interface{}{
						"k1": "v1",
						"k2": "v2",
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"k2": "v2-new",
						"k3": "v3",
					},
				},
			},
			result: `{"data":{"k2":"v2-new","k3":"v3"},"metadata":{"annotations":{"last-applied/config":"{\"apiVersion\":\"v1\",\"data\":{\"k2\":\"v2-new\",\"k3\":\"v3\"},\"kind\":\"Foo\",\"metadata\":{\"name\":\"test\",\"namespace\":\"default\"}}"}}}`,
		},
		"built-in resources": {
			PatchAction: &patch.PatchAction{},
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"k1": "v1",
						"k2": "v2",
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"k2": "v2-new",
						"k3": "v3",
					},
				},
			},
			result: `{"data":{"k2":"v2-new","k3":"v3"}}`,
		},
		"err case": {
			PatchAction: &patch.PatchAction{},
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"test": "Test",
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"k2": "v2-new",
						"k3": "v3",
					},
				},
			},
			result:  `{"data":{"k2":"v2-new","k3":"v3"}}`,
			wantErr: "precondition failed for",
		},
		"err: cannot marshal current": {
			PatchAction: &patch.PatchAction{},
			current: &object.UnknownObject{
				Chan: make(chan int),
			},
			modified: &object.UnknownObject{},
			wantErr:  "json: unsupported type: chan int",
		},
	}

	for caseName, tc := range cases {
		t.Run(caseName, func(t *testing.T) {
			r := require.New(t)
			result, err := patch.ThreeWayMergePatch(tc.current, tc.modified, tc.PatchAction)
			if tc.wantErr != "" {
				r.Contains(err.Error(), tc.wantErr)
				return
			}
			r.NoError(err)
			data, err := result.Data(nil)
			r.NoError(err)
			r.Equal(tc.result, string(data))
		})
	}
}

func TestAddLastAppliedConfiguration(t *testing.T) {
	cases := map[string]struct {
		reason  string
		obj     runtime.Object
		wantErr string
	}{
		"CustomResource": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"k2": "v2-new",
						"k3": "v3",
					},
				},
			},
		},
		"err": {
			obj: &object.UnknownObject{
				Chan: make(chan int),
			},
			wantErr: "json: unsupported type: chan int",
		},
	}

	for caseName, tc := range cases {
		t.Run(caseName, func(t *testing.T) {
			r := require.New(t)
			err := patch.AddLastAppliedConfiguration(tc.obj, "last-applied/config", "last-applied/time")
			if tc.wantErr != "" {
				r.Equal(err.Error(), tc.wantErr)
				return
			}
			r.NoError(err)
			metadataAccessor := meta.NewAccessor()
			annos, err := metadataAccessor.Annotations(tc.obj)
			r.NoError(err)
			r.NotEmpty(annos["last-applied/config"])
			r.NotEmpty(annos["last-applied/time"])
		})
	}
}

func TestGetOriginalConfiguration(t *testing.T) {
	objNoAnno := &unstructured.Unstructured{}
	objNoAnno.SetAnnotations(make(map[string]string))

	cases := map[string]struct {
		reason     string
		obj        runtime.Object
		wantConfig string
		wantErr    error
	}{
		"NoAnnotations": {
			reason: "No error should be returned if cannot find last-applied-config annotaion",
			obj:    &unstructured.Unstructured{},
		},
		"LastAppliedConfigAnnotationNotFound": {
			reason: "No error should be returned if cannot find last-applied-config annotaion",
			obj:    objNoAnno,
		},
	}

	for caseName, tc := range cases {
		t.Run(caseName, func(t *testing.T) {
			config := patch.GetOriginalConfiguration(tc.obj, "last-applied/config")
			if diff := cmp.Diff(tc.wantConfig, string(config)); diff != "" {
				t.Errorf("\n%s\ngetModifiedConfig(...): -want , +got \n%s\n", tc.reason, diff)
			}
		})
	}
}
