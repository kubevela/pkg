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

package client_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	velaclient "github.com/kubevela/pkg/controller/client"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/test/bootstrap"
	"github.com/kubevela/pkg/util/test/tester"
)

func TestMulticluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Run client package test")
}

var _ = bootstrap.InitKubeBuilderForTest()

var _ = Describe("Test clients", func() {

	It("Test default controller client", func() {
		cfg := singleton.KubeConfig.Get()

		_cache, err := cache.New(cfg, cache.Options{})
		Ω(err).To(Succeed())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			Ω(_cache.Start(ctx)).To(Succeed())
		}()

		velaclient.CachedGVKs = "Deployment.apps.v1"
		_client, err := velaclient.DefaultNewControllerClient(_cache, cfg, client.Options{}, &corev1.Secret{})
		Ω(err).To(Succeed())
		tester.TestClientFunctions(_client)
		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		}}
		_ctx := context.Background()
		Ω(_client.Get(_ctx, client.ObjectKey{Namespace: "default", Name: "example"}, obj)).To(Satisfy(kerrors.IsNotFound))
		objs := &unstructured.UnstructuredList{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMapList",
		}}
		Ω(_client.List(_ctx, objs)).To(Succeed())
		gvk, err := _client.GroupVersionKindFor(obj)
		Ω(gvk).To(Equal(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}))
		Ω(err).To(Succeed())
		namespaced, err := _client.IsObjectNamespaced(obj)
		Ω(namespaced).To(BeTrue())
		Ω(err).To(Succeed())
	})
})
