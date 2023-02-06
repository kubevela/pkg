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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pkgclient "github.com/kubevela/pkg/controller/client"
	"github.com/kubevela/pkg/meta"
	"github.com/kubevela/pkg/util/k8s"
)

func TestNamespace(t *testing.T) {
	ctx := context.Background()
	base := fake.NewClientBuilder().Build()
	cli := &pkgclient.DelegatingHandlerClient{
		Client: base,
		Getter: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			switch key.Name {
			case "exist":
				return nil
			case "err":
				return fmt.Errorf("")
			case "notfound":
				return apierrors.NewNotFound(schema.GroupResource{}, "")
			default:
				return base.Get(ctx, key, obj)
			}
		},
	}
	require.NoError(t, k8s.EnsureNamespace(ctx, cli, "exist"))
	require.Error(t, k8s.EnsureNamespace(ctx, cli, "err"))
	require.NoError(t, k8s.EnsureNamespace(ctx, cli, "create"))

	require.NoError(t, k8s.ClearNamespace(ctx, cli, "notfound"))
	require.Error(t, k8s.ClearNamespace(ctx, cli, "err"))
	require.NoError(t, k8s.ClearNamespace(ctx, cli, "create"))

	require.Equal(t, meta.NamespaceVelaSystem, k8s.GetRuntimeNamespace())
}
