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

package k8s

import (
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// GetKindForObject extract kind from runtime.Object
// If kind already set, directly use it.
// Otherwise, use reflection to retrieve it from the struct type.
// If trimList set to true, the returned kind will be trimmed.
func GetKindForObject(obj runtime.Object, trimList bool) string {
	o := obj.GetObjectKind().GroupVersionKind()
	kind := o.Kind
	if kind == "" {
		if t := reflect.TypeOf(obj); t.Kind() == reflect.Ptr {
			kind = t.Elem().Name()
		} else {
			kind = t.Name()
		}
	}
	if trimList {
		kind = strings.TrimSuffix(kind, "List")
	}
	return kind
}

// IsUnstructuredObject check if runtime.Object is unstructured
func IsUnstructuredObject(obj runtime.Object) bool {
	_, isUnstructured := obj.(*unstructured.Unstructured)
	_, isUnstructuredList := obj.(*unstructured.UnstructuredList)
	return isUnstructured || isUnstructuredList
}
