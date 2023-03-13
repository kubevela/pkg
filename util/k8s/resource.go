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

package k8s

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resource .
type Resource struct {
	Group     string `json:"group"`
	Resource  string `json:"resource"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func GetGVKFromResource(ctx context.Context, cli client.Client, resource Resource) (schema.GroupVersionKind, error) {
	mapper := cli.RESTMapper()
	gvks, err := mapper.KindsFor(schema.GroupVersionResource{Group: resource.Group, Resource: resource.Resource})
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, errors.Errorf("no kind found for resource %s", resource)
	}
	return gvks[0], nil
}

func GetUnstructuredFromResource(ctx context.Context, cli client.Client, resource Resource) (*unstructured.Unstructured, error) {
	gvk, err := GetGVKFromResource(ctx, cli, resource)
	if err != nil {
		return nil, err
	}
	isNamespaced, err := IsGroupVersionKindNamespaceScoped(cli.RESTMapper(), gvk)
	if err != nil {
		return nil, err
	}
	un := &unstructured.Unstructured{}
	un.SetGroupVersionKind(gvk)
	if isNamespaced {
		un.SetNamespace(resource.Namespace)
	}
	if err := cli.Get(ctx, client.ObjectKey{Name: resource.Name, Namespace: resource.Namespace}, un); err != nil {
		return nil, err
	}
	return un, nil
}

// IsGroupVersionKindNamespaceScoped check if the target GroupVersionKind is namespace scoped resource
func IsGroupVersionKindNamespaceScoped(mapper meta.RESTMapper, gvk schema.GroupVersionKind) (bool, error) {
	mappings, err := mapper.RESTMappings(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, err
	}
	if len(mappings) == 0 {
		return false, fmt.Errorf("unable to fund the mappings for gvk %s", gvk)
	}
	return mappings[0].Scope.Name() == meta.RESTScopeNameNamespace, nil
}
