/*
Copyright 2026 The KubeVela Authors.

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

package multicluster_test

import (
	"context"

	. "github.com/onsi/gomega"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/rand"
)

const applyFieldOwner = client.FieldOwner("multicluster-test")

func deploymentApply(namespace, name string) *appsv1ac.DeploymentApplyConfiguration {
	labels := map[string]string{"app": "apply"}
	return appsv1ac.Deployment(name, namespace).WithSpec(appsv1ac.DeploymentSpec().
		WithReplicas(1).
		WithSelector(metav1ac.LabelSelector().WithMatchLabels(labels)).
		WithTemplate(corev1ac.PodTemplateSpec().
			WithLabels(labels).
			WithSpec(corev1ac.PodSpec().WithContainers(corev1ac.Container().WithName("test").WithImage("test")))))
}

func deploymentStatusApply(namespace, name string) *appsv1ac.DeploymentApplyConfiguration {
	return appsv1ac.Deployment(name, namespace).WithStatus(appsv1ac.DeploymentStatus().WithObservedGeneration(1))
}

// testApplyFunctions server-side applies a Deployment and its status through
// every Apply entry point of the client.
func testApplyFunctions(ctx context.Context, c client.Client) {
	namespace, name := "test-"+rand.RandomString(4), "apply"
	Ω(k8s.EnsureNamespace(ctx, c, namespace)).To(Succeed())
	deploy := deploymentApply(namespace, name)
	Ω(c.Apply(ctx, deploy, applyFieldOwner)).To(Succeed())
	Ω(deploy.UID).NotTo(BeNil(), "the apply response should be decoded back into the apply configuration")
	Ω(c.Status().Apply(ctx, deploymentStatusApply(namespace, name), applyFieldOwner)).To(Succeed())
	Ω(c.SubResource("status").Apply(ctx, deploymentStatusApply(namespace, name), applyFieldOwner)).To(Succeed())
	Ω(k8s.ClearNamespace(ctx, c, namespace)).To(Succeed())
}
