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

package kube_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/cue/cuex/providers/kube"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/slices"
)

func newConfigMap(name string, namespace string, label string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"label": label},
		},
	}
}

func TestKube(t *testing.T) {
	cli := fake.NewClientBuilder().WithObjects(
		newConfigMap("a", "x", "1"),
		newConfigMap("b", "y", "1"),
		newConfigMap("c", "x", "1"),
		newConfigMap("d", "x", "2"),
	).Build()
	singleton.KubeClient.Set(cli)
	ctx := context.Background()
	v, err := kube.Get(ctx, &kube.GetVar{Value: &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "a",
			"namespace": "x",
		},
	}}})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"label": "1"}, v.Value.GetLabels())

	vs, err := kube.List(ctx, &kube.ListVar{
		Filter: &kube.ListFilter{
			Namespace:      "x",
			MatchingLabels: map[string]string{"label": "1"},
		},
		List: &unstructured.UnstructuredList{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(vs.List.Items))
	require.True(t, slices.Index(vs.List.Items, func(i unstructured.Unstructured) bool { return i.GetName() == "a" }) >= 0)
	require.True(t, slices.Index(vs.List.Items, func(i unstructured.Unstructured) bool { return i.GetName() == "c" }) >= 0)
}
