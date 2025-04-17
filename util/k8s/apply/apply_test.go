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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/util/jsonutil"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/k8s/apply"
	"github.com/kubevela/pkg/util/k8s/patch"
)

type FakeOptions struct {
	dryRun         bool
	applyErr       bool
	createErr      bool
	updateErr      bool
	updateStrategy string
}

func (in *FakeOptions) GetPatchAction() patch.PatchAction {
	return patch.PatchAction{
		UpdateAnno:            true,
		AnnoLastAppliedConfig: "lac",
		AnnoLastAppliedTime:   "lat",
	}
}

func (in *FakeOptions) PreUpdate(existing, desired client.Object) error {
	if in.updateErr {
		return fmt.Errorf("pre-update error")
	}
	return nil
}

func (in *FakeOptions) PreCreate(desired client.Object) error {
	if in.createErr {
		return fmt.Errorf("pre-create error")
	}
	return nil
}

func (in *FakeOptions) PreApply(desired client.Object) error {
	if in.applyErr {
		return fmt.Errorf("pre-apply error")
	}
	return patch.AddLastAppliedConfiguration(desired, "lac", "lat")
}

func (in *FakeOptions) DryRun() apply.DryRunOption {
	return apply.DryRunOption(in.dryRun)
}

func (in *FakeOptions) GetUpdateStrategy(existing, desired client.Object) (apply.UpdateStrategy, error) {
	switch in.updateStrategy {
	case "patch":
		return apply.Patch, nil
	case "replace":
		return apply.Replace, nil
	case "recreate":
		return apply.Recreate, nil
	case "skip":
		return apply.Skip, nil
	default:
		return apply.Skip, fmt.Errorf("unexpected update-strategy")
	}
}

var _ apply.Options = &FakeOptions{}
var _ apply.PreApplyHook = &FakeOptions{}
var _ apply.PreCreateHook = &FakeOptions{}
var _ apply.PreUpdateHook = &FakeOptions{}
var _ apply.PatchActionProvider = &FakeOptions{}

func TestApply(t *testing.T) {
	testCases := map[string]struct {
		Existing           client.Object
		Desired            client.Object
		Expected           client.Object
		Options            apply.Options
		HasLastAppliedAnno bool
		Err                string
	}{
		"pre-apply err": {
			Desired: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Options: &FakeOptions{applyErr: true},
			Err:     "pre-apply err",
		},
		"pre-create err": {
			Desired: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Options: &FakeOptions{createErr: true},
			Err:     "pre-create err",
		},
		"pre-update err": {
			Existing: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{}},
			Desired:  &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Options:  &FakeOptions{updateErr: true},
			Err:      "pre-update err",
		},
		"invalid-update-strategy": {
			Existing: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{}},
			Desired:  &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Options:  &FakeOptions{updateStrategy: "bad"},
			Err:      "unexpected update-strategy",
		},
		"create": {
			Desired:            &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Expected:           &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Options:            &FakeOptions{},
			HasLastAppliedAnno: true,
		},
		"skip-update": {
			Existing: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{}},
			Desired:  &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Expected: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{}},
			Options:  &FakeOptions{updateStrategy: "skip"},
		},
		"patch": {
			Existing:           &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"old": "0"}},
			Desired:            &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Expected:           &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"old": "0", "key": "value"}},
			Options:            &FakeOptions{updateStrategy: "patch"},
			HasLastAppliedAnno: true,
		},
		"empty-patch": {
			Existing: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default", Annotations: map[string]string{"lac": `{"apiVersion":"v1","data":{"old":"0"},"kind":"ConfigMap","metadata":{"creationTimestamp":null,"name":"default","namespace":"default"}}`}}, Data: map[string]string{"old": "0"}},
			Desired:  &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"old": "0"}},
			Expected: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default", Annotations: map[string]string{"lac": `{"apiVersion":"v1","data":{"old":"0"},"kind":"ConfigMap","metadata":{"creationTimestamp":null,"name":"default","namespace":"default"}}`}}, Data: map[string]string{"old": "0"}},
			Options:  &FakeOptions{updateStrategy: "patch"},
		},
		"recreate": {
			Existing:           &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"old": "0"}},
			Desired:            &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Expected:           &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Options:            &FakeOptions{updateStrategy: "recreate"},
			HasLastAppliedAnno: true,
		},
		"recreate-dryrun": {
			Existing: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"old": "0"}},
			Desired:  &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Expected: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"old": "0"}},
			Options:  &FakeOptions{updateStrategy: "recreate", dryRun: true},
		},
		"replace": {
			Existing:           &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"old": "0"}},
			Desired:            &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Expected:           &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}, Data: map[string]string{"key": "value"}},
			Options:            &FakeOptions{updateStrategy: "replace"},
			HasLastAppliedAnno: true,
		},
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			cli := fake.NewClientBuilder().Build()
			if tt.Existing != nil {
				require.NoError(t, cli.Create(ctx, tt.Existing))
			}

			err := apply.Apply(ctx, cli, tt.Desired, tt.Options)
			if len(tt.Err) > 0 {
				require.ErrorContains(t, err, tt.Err)
				return
			}
			require.NoError(t, err)

			if tt.Expected != nil {
				key := client.ObjectKeyFromObject(tt.Desired)
				un := &unstructured.Unstructured{}
				un.SetGroupVersionKind(tt.Desired.GetObjectKind().GroupVersionKind())
				require.NoError(t, cli.Get(ctx, key, un))
				un.SetResourceVersion("")
				if tt.HasLastAppliedAnno {
					require.NotEmpty(t, k8s.GetAnnotation(un, "lac"))
					require.NotEmpty(t, k8s.GetAnnotation(un, "lat"))
					un.SetAnnotations(nil)
				}
				_un, _ := jsonutil.AsType[map[string]interface{}](un)
				delete(*_un, "apiVersion")
				delete(*_un, "kind")
				_exp, _ := jsonutil.AsType[map[string]interface{}](tt.Expected)
				require.Equal(t, _exp, _un)
			}
		})
	}
}
