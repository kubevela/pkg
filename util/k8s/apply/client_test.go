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

package apply_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/util/jsonutil"
	"github.com/kubevela/pkg/util/k8s/apply"
)

func TestApplyClientUnstructured(t *testing.T) {
	cli := apply.Client{Client: fake.NewClientBuilder().Build()}
	deploy, err := jsonutil.AsType[unstructured.Unstructured](&appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(1))},
		Status:     appsv1.DeploymentStatus{Replicas: 1},
	})
	require.NoError(t, err)
	_ctx := context.Background()
	require.NoError(t, cli.Create(_ctx, deploy))
	require.EqualValues(t, int64(1), deploy.Object["spec"].(map[string]interface{})["replicas"])
	require.EqualValues(t, int64(1), deploy.Object["status"].(map[string]interface{})["replicas"])

	deploy.Object["spec"].(map[string]interface{})["replicas"] = 3
	deploy.Object["status"].(map[string]interface{})["replicas"] = 3
	require.NoError(t, cli.Update(_ctx, deploy))

	_deploy := &unstructured.Unstructured{Object: map[string]interface{}{}}
	_deploy.SetAPIVersion("apps/v1")
	_deploy.SetKind("Deployment")
	require.NoError(t, cli.Get(_ctx, types.NamespacedName{Namespace: "default", Name: "test"}, _deploy))
	require.Equal(t, int64(3), _deploy.Object["spec"].(map[string]interface{})["replicas"])
	require.Equal(t, int64(3), _deploy.Object["status"].(map[string]interface{})["replicas"])

	p := client.RawPatch(types.JSONPatchType, []byte(`[{"op":"replace","path":"/spec/replicas","value":5},{"op":"replace","path":"/status/replicas","value":5}]`))
	require.NoError(t, cli.Patch(_ctx, _deploy, p))
	require.Equal(t, int64(5), _deploy.Object["spec"].(map[string]interface{})["replicas"])
	require.Equal(t, int64(5), _deploy.Object["status"].(map[string]interface{})["replicas"])
}

func TestApplyClientStructured(t *testing.T) {
	cli := apply.Client{Client: fake.NewClientBuilder().Build()}
	deploy := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(1))},
		Status:     appsv1.DeploymentStatus{Replicas: 1},
	}
	_ctx := context.Background()
	require.NoError(t, cli.Create(_ctx, deploy, client.DryRunAll))
	require.NoError(t, cli.Create(_ctx, deploy))
	require.Equal(t, ptr.To(int32(1)), deploy.Spec.Replicas)
	require.Equal(t, int32(1), deploy.Status.Replicas)

	deploy.Spec.Replicas = ptr.To(int32(3))
	deploy.Status.Replicas = 3
	require.NoError(t, cli.Update(_ctx, deploy))

	_deploy := &appsv1.Deployment{}
	require.NoError(t, cli.Get(_ctx, types.NamespacedName{Namespace: "default", Name: "test"}, _deploy))
	require.Equal(t, ptr.To(int32(3)), _deploy.Spec.Replicas)
	require.Equal(t, int32(3), _deploy.Status.Replicas)

	p := client.RawPatch(types.JSONPatchType, []byte(`[{"op":"replace","path":"/spec/replicas","value":5},{"op":"replace","path":"/status/replicas","value":5}]`))
	require.NoError(t, cli.Patch(_ctx, _deploy, p))
	require.Equal(t, ptr.To(int32(5)), _deploy.Spec.Replicas)
	require.Equal(t, int32(5), _deploy.Status.Replicas)
}

func TestApplyClientBad(t *testing.T) {
	cli := apply.Client{Client: fake.NewClientBuilder().Build()}
	_ctx := context.Background()
	require.Error(t, cli.Create(_ctx, nil))
	require.Error(t, cli.Update(_ctx, nil))
	p := client.RawPatch(types.JSONPatchType, []byte(``))
	require.Error(t, cli.Patch(_ctx, nil, p))
}
