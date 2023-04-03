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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/rand"
)

// TestClientFunctions a list of functions to test wrapped client
func TestClientFunctions(c client.Client) {
	ctx := context.Background()
	namespace, name := "test-"+rand.RandomString(4), "fake"
	Ω(k8s.EnsureNamespace(ctx, c, namespace)).To(Succeed())
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "test"}}},
			},
		},
	}
	Ω(c.Create(ctx, deploy)).To(Succeed())
	Ω(c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, deploy)).To(Succeed())
	Ω(c.List(ctx, &corev1.ServiceList{})).To(Succeed())
	Ω(c.Update(ctx, deploy)).To(Succeed())
	Ω(c.Patch(ctx, deploy, client.Merge)).To(Succeed())
	Ω(c.Status().Update(ctx, deploy)).To(Succeed())
	Ω(c.Status().Patch(ctx, deploy, client.Merge)).To(Succeed())
	Ω(c.SubResource("status").Get(ctx, deploy, deploy)).To(Succeed())
	Ω(c.SubResource("status").Update(ctx, deploy)).To(Succeed())
	Ω(c.SubResource("status").Patch(ctx, deploy, client.Merge)).To(Succeed())
	Ω(c.Delete(ctx, deploy)).To(Succeed())
	Ω(c.DeleteAllOf(ctx, &corev1.ConfigMap{}, client.InNamespace(namespace))).To(Succeed())
	Ω(c.Scheme().IsGroupRegistered("apps")).To(BeTrue())
	_, err := c.RESTMapper().ResourceSingularizer("configmaps")
	Ω(err).To(Succeed())
	Ω(k8s.ClearNamespace(ctx, c, namespace)).To(Succeed())
}
