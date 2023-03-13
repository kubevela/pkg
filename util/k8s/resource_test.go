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

package k8s_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/singleton"
)

func TestGetGVKFromResource(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "", Kind: "Namespace"}, meta.RESTScopeRoot)
	cli := fake.NewClientBuilder().WithRESTMapper(mapper).Build()
	singleton.KubeClient.Set(cli)
	ctx := context.Background()
	testcases := map[string]struct {
		resource    k8s.ResourceIdentifier
		expectedErr string
	}{
		"valid": {
			resource: k8s.ResourceIdentifier{Group: "apps", Resource: "Deployment"},
		},
		"invalid": {
			resource:    k8s.ResourceIdentifier{Group: "invalid", Resource: "Deployment"},
			expectedErr: "no matches for invalid",
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			_, err := k8s.GetGVKFromResource(ctx, tc.resource)
			if tc.expectedErr != "" {
				r.Contains(err.Error(), tc.expectedErr)
				return
			}
			r.NoError(err)
		})
	}
}

func TestIsGroupVersionKindNamespaceScoped(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	testcases := map[string]struct {
		gvk         schema.GroupVersionKind
		expected    bool
		expectedErr string
	}{
		"true": {
			gvk:      schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			expected: true,
		},
		"false": {
			gvk:      schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"},
			expected: false,
		},
		"invalid": {
			gvk:         schema.GroupVersionKind{Group: "invalid", Version: "v1", Kind: "Deployment"},
			expectedErr: "no matches for kind",
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			result, err := k8s.IsGVKNamespaced(tc.gvk, mapper)
			if tc.expectedErr != "" {
				r.Contains(err.Error(), tc.expectedErr)
				return
			}
			r.NoError(err)
			r.Equal(tc.expected, result)
		})
	}
}

func TestGetUnstructuredFromResource(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "", Kind: "Namespace"}, meta.RESTScopeRoot)
	cli := fake.NewClientBuilder().WithObjects(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: "default",
			},
		},
	).WithRESTMapper(mapper).Build()
	singleton.KubeClient.Set(cli)
	ctx := context.Background()
	testcases := map[string]struct {
		resource    k8s.ResourceIdentifier
		expectedErr string
	}{
		"valid": {
			resource: k8s.ResourceIdentifier{Group: "apps", Resource: "Deployment", Name: "test-deploy", Namespace: "default"},
		},
		"not-found": {
			resource:    k8s.ResourceIdentifier{Group: "apps", Resource: "Deployment", Name: "not-found", Namespace: "default"},
			expectedErr: "not found",
		},
		"invalid": {
			resource:    k8s.ResourceIdentifier{Group: "invalid", Resource: "Deployment"},
			expectedErr: "no matches for invalid",
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			_, err := k8s.GetUnstructuredFromResource(ctx, tc.resource)
			if tc.expectedErr != "" {
				r.Contains(err.Error(), tc.expectedErr)
				return
			}
			r.NoError(err)
		})
	}
}
