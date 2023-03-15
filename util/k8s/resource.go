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

	"github.com/kubevela/pkg/util/singleton"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceIdentifier .
type ResourceIdentifier struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

// GetGVKFromResource returns the GVK for the provided resource identifier.
func GetGVKFromResource(resource ResourceIdentifier) (schema.GroupVersionKind, error) {
	gv, err := schema.ParseGroupVersion(resource.APIVersion)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	gvk := gv.WithKind(resource.Kind)
	return gvk, nil
}

// GetUnstructuredFromResource returns an unstructured object for the provided resource identifier.
func GetUnstructuredFromResource(ctx context.Context, resource ResourceIdentifier) (*unstructured.Unstructured, error) {
	gvk, err := GetGVKFromResource(resource)
	if err != nil {
		return nil, err
	}
	isNamespaced, err := IsGVKNamespaced(gvk, singleton.RESTMapper.Get())
	if err != nil {
		return nil, err
	}
	un := &unstructured.Unstructured{}
	un.SetGroupVersionKind(gvk)
	if isNamespaced {
		un.SetNamespace(resource.Namespace)
	}
	if err := singleton.KubeClient.Get().Get(ctx, client.ObjectKey{Name: resource.Name, Namespace: resource.Namespace}, un); err != nil {
		return nil, err
	}
	return un, nil
}

// IsGVKNamespaced returns true if the object having the provided
// GVK is namespace scoped.
func IsGVKNamespaced(gvk schema.GroupVersionKind, restmapper meta.RESTMapper) (bool, error) {
	mappings, err := restmapper.RESTMappings(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, err
	}
	if len(mappings) == 0 {
		return false, fmt.Errorf("unable to fund the mappings for gvk %s", gvk)
	}
	return mappings[0].Scope.Name() == meta.RESTScopeNameNamespace, nil
}
