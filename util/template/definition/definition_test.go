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

package definition_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/kubevela/pkg/util/template/definition"
)

func TestLoadMainTemplate(t *testing.T) {
	sc := scheme.Scheme
	_ = v1alpha1.AddToScheme(sc)
	cli := fake.NewFakeClientWithScheme(sc)
	ctx := context.Background()
	r := require.New(t)
	err := cli.Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vela-system",
		},
	})
	r.NoError(err)
	loader := definition.NewTemplateLoader(ctx, cli)

	// case: no type
	_, err = loader.LoadTemplate(ctx, "no-type")
	r.Error(err)

	// case: not found
	_, err = loader.LoadTemplate(ctx, "not-found", definition.WithType("action"))
	r.Error(err)

	// case: invalid type
	err = cli.Create(ctx, &v1alpha1.Definition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: v1alpha1.DefinitionSpec{
			Type: "action",
		},
	})
	r.NoError(err)
	nsCtx := definition.WithNamespace(ctx, "default")
	_, err = loader.LoadTemplate(nsCtx, "test", definition.WithType("invalid-type"))
	r.Error(err)

	// case: no main.cue in template
	_, err = loader.LoadTemplate(nsCtx, "test", definition.WithType("action"))
	r.Error(err)

	// case: main.cue in template
	err = cli.Create(ctx, &v1alpha1.Definition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "valid",
			Namespace: "default",
		},
		Spec: v1alpha1.DefinitionSpec{
			Type: "action",
			Templates: map[string]string{
				"main.cue": "main",
			},
		},
	})
	r.NoError(err)
	template, err := loader.LoadTemplate(nsCtx, "valid", definition.WithType("action"))
	r.NoError(err)
	s := template.Compile()
	r.Equal("main", s)
}
