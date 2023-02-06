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
	"context"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/meta"
)

// EnsureNamespace ensure namespace existence. If not, create it.
func EnsureNamespace(ctx context.Context, c client.Client, ns string) error {
	namespace := &corev1.Namespace{}
	err := c.Get(ctx, types.NamespacedName{Name: ns}, namespace)
	switch {
	case client.IgnoreNotFound(err) != nil:
		return err
	case err == nil:
		return nil
	default:
		namespace.SetName(ns)
		return c.Create(ctx, namespace)
	}
}

// ClearNamespace clear namespace if exists.
func ClearNamespace(ctx context.Context, c client.Client, ns string) error {
	namespace := &corev1.Namespace{}
	err := c.Get(ctx, types.NamespacedName{Name: ns}, namespace)
	switch {
	case err != nil && errors.IsNotFound(err):
		return nil
	case err != nil:
		return err
	}
	return client.IgnoreNotFound(c.Delete(ctx, namespace))
}

// GetRuntimeNamespace get namespace of the current running pod, fall back to default vela system
func GetRuntimeNamespace() string {
	ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return meta.NamespaceVelaSystem
	}
	return string(ns)
}
