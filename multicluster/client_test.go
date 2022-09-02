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

package multicluster_test

import (
	"context"
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/test/kubebuilder"
)

var _ = Describe("Test multicluster client", func() {
	cfg := kubebuilder.GetConfig()

	It("Test create client", func() {
		By("New native client")
		_, err := multicluster.NewClient(cfg, multicluster.ClientOptions{})
		Ω(err).To(Succeed())

		By("New gated client")
		file, err := ioutil.TempFile("/tmp", "config")
		Ω(err).To(Succeed())
		defer func() {
			Ω(os.Remove(file.Name())).To(Succeed())
		}()
		Ω(ioutil.WriteFile(file.Name(), cfg.CAData, 0600)).To(Succeed())
		c, err := multicluster.NewClient(cfg, multicluster.ClientOptions{
			Options: client.Options{Scheme: scheme.Scheme},
			ClusterGateway: multicluster.ClusterGatewayClientOptions{
				URL:    cfg.Host,
				CAFile: file.Name(),
			},
		})
		Ω(err).To(Succeed())

		By("Test basic functions")
		ctx := context.Background()
		namespace, name := "default", "fake"
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
		Ω(c.Scheme().IsGroupRegistered("cluster.core.oam.dev")).To(BeTrue())
		_, err = c.RESTMapper().ResourceSingularizer("configmaps")
		Ω(err).To(Succeed())

		By("Client with args")
		multicluster.AddClusterGatewayClientFlags(pflag.CommandLine)
		_, err = multicluster.NewDefaultClient(cfg, client.Options{})
		Ω(err).To(Succeed())
	})
})
