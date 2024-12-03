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

package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	testobj "github.com/kubevela/pkg/util/test/object"
)

func TestDelegatingReaderSchemeErr(t *testing.T) {
	dr := &delegatingReader{scheme: scheme.Scheme}
	require.Error(t, dr.Get(context.Background(), client.ObjectKey{}, &testobj.UnknownObject{}))
	require.Error(t, dr.List(context.Background(), &testobj.UnknownObjectList{}))
}

func TestDelegatingClient(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	c := fake.NewClientBuilder().
		WithRESTMapper(mapper).
		Build()
	_client := &delegatingClient{
		client: c,
	}
	ok, err := _client.IsObjectNamespaced(&corev1.ConfigMap{})
	require.NoError(t, err)
	require.True(t, ok)
	gvk, err := _client.GroupVersionKindFor(&corev1.ConfigMap{})
	require.NoError(t, err)
	require.Equal(t, schema.GroupVersionKind{Kind: "ConfigMap", Version: "v1"}, gvk)
}
