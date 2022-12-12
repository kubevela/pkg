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
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/test/kubebuilder"
	"github.com/kubevela/pkg/util/test/tester"
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
		Ω(os.WriteFile(file.Name(), cfg.CAData, 0600)).To(Succeed())
		c, err := multicluster.NewClient(cfg, multicluster.ClientOptions{
			Options: client.Options{Scheme: scheme.Scheme},
			ClusterGateway: multicluster.ClusterGatewayClientOptions{
				URL:    cfg.Host,
				CAFile: file.Name(),
			},
		})
		Ω(err).To(Succeed())

		By("Test basic functions")
		tester.TestClientFunctions(c)

		By("Test without ca")
		c, err = multicluster.NewClient(cfg, multicluster.ClientOptions{
			Options:        client.Options{Scheme: scheme.Scheme},
			ClusterGateway: multicluster.ClusterGatewayClientOptions{URL: cfg.Host},
		})
		Ω(err).To(Succeed())
		tester.TestClientFunctions(c)

		By("Client with args")
		multicluster.AddClusterGatewayClientFlags(pflag.CommandLine)
		_, err = multicluster.NewDefaultClient(cfg, client.Options{})
		Ω(err).To(Succeed())
	})
})
