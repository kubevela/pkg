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

package tester

import (
	"context"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/rand"
)

func TestClientFunctions(c client.Client) {
	ctx := context.Background()
	namespace, name := "test-"+rand.RandomString(4), "fake"
	Ω(k8s.EnsureNamespace(ctx, c, namespace)).To(Succeed())
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "example", Port: 6443}},
		},
	}
	Ω(c.Create(ctx, svc)).To(Succeed())
	Ω(c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, svc)).To(Succeed())
	Ω(c.List(ctx, &corev1.ServiceList{})).To(Succeed())
	Ω(c.Update(ctx, svc)).To(Succeed())
	Ω(c.Patch(ctx, svc, client.Merge)).To(Succeed())
	Ω(c.Status().Update(ctx, svc)).To(Succeed())
	Ω(c.Status().Patch(ctx, svc, client.Merge)).To(Succeed())
	Ω(c.Delete(ctx, svc)).To(Succeed())
	Ω(c.DeleteAllOf(ctx, &corev1.ConfigMap{}, client.InNamespace(namespace))).To(Succeed())
	Ω(c.Scheme().IsGroupRegistered("apps")).To(BeTrue())
	_, err := c.RESTMapper().ResourceSingularizer("configmaps")
	Ω(err).To(Succeed())
	Ω(k8s.ClearNamespace(ctx, c, namespace)).To(Succeed())
}
