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

package client_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	velaclient "github.com/kubevela/pkg/controller/client"
)

func TestDelegatingHandlerClient(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	err := fmt.Errorf("injected")
	_client := &velaclient.DelegatingHandlerClient{
		Client: c,
		Getter: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			return err
		},
		Lister: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			return err
		},
	}
	ctx := context.Background()
	require.ErrorIs(t, err, _client.Get(ctx, client.ObjectKey{
		Namespace: "default",
		Name:      "example",
	}, &corev1.ConfigMap{}))
	require.ErrorIs(t, err, _client.List(ctx, &corev1.ConfigMapList{}))

	_client = &velaclient.DelegatingHandlerClient{Client: c}
	require.True(t, kerrors.IsNotFound(_client.Get(ctx, client.ObjectKey{
		Namespace: "default",
		Name:      "example",
	}, &corev1.ConfigMap{})))
	require.NoError(t, _client.List(ctx, &corev1.ConfigMapList{}))
}
