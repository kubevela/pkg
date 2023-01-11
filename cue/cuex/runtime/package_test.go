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

package runtime_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubevela/pkg/apis/cuex/v1alpha1"
	"github.com/kubevela/pkg/cue/cuex/runtime"
)

func TestInternalPackage(t *testing.T) {
	fn := runtime.GenericProviderFn[value, value](func(ctx context.Context, t *value) (*value, error) {
		return t, nil
	})
	tpl := `
		package test
		#Fn: {
			input: string
			output?: string
		}
	`
	pkg, err := runtime.NewInternalPackage("ext/test", tpl, map[string]runtime.ProviderFn{"fn": fn})
	require.NoError(t, err)
	require.NotNil(t, pkg.GetProviderFn("fn"))
	require.Nil(t, pkg.GetProviderFn("unknown"))
	require.Equal(t, "ext/test", pkg.GetName())
	require.Equal(t, "vela/ext/test", pkg.GetPath())
	require.Equal(t, []string{tpl}, pkg.GetTemplates())
	require.Equal(t, 1, len(pkg.GetImports()))
	require.Equal(t, "test", pkg.GetImports()[0].PkgName)
}

func newTestPackage(endpoint string) (*v1alpha1.Package, string) {
	tpl := `
		package test
		#Fn: {
			#do: "test"
			#provider: "test"
			input: string
			output?: string
		}
	`
	return &v1alpha1.Package{
		ObjectMeta: metav1.ObjectMeta{Name: "ext-test"},
		Spec: v1alpha1.PackageSpec{
			Path: "remote/ext/test",
			Provider: &v1alpha1.Provider{
				Protocol: v1alpha1.ProtocolHTTP,
				Endpoint: endpoint,
			},
			Templates: map[string]string{
				"test.cue": tpl,
			},
		},
	}, tpl
}

func TestExternalPackage(t *testing.T) {
	server := newTestServer()
	defer server.Close()
	_pkg, tpl := newTestPackage(server.URL)
	pkg, err := runtime.NewExternalPackage(_pkg)
	require.NoError(t, err)
	require.NotNil(t, pkg.GetProviderFn("fn"))
	require.Equal(t, "ext-test", pkg.GetName())
	require.Equal(t, "remote/ext/test", pkg.GetPath())
	require.Equal(t, []string{tpl}, pkg.GetTemplates())
	require.Equal(t, 1, len(pkg.GetImports()))
	require.Equal(t, "test", pkg.GetImports()[0].PkgName)
}
