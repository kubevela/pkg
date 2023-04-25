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

package reconciler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"github.com/kubevela/pkg/controller/reconciler"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/test/bootstrap"
)

func TestReconciler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Run reconciler package test")
}

var _ = bootstrap.InitKubeBuilderForTest()

var _ = Describe("Test clients", func() {

	It("Test default controller client", func() {
		cfg := singleton.KubeConfig.Get()
		mgr, err := controllerruntime.NewManager(cfg, controllerruntime.Options{})
		Ω(err).To(Succeed())

		const path = "/trigger"
		ch := reconciler.RegisterTriggerHandler(mgr, path, 1024)
		svr := httptest.NewServer(mgr.GetWebhookServer().WebhookMux)
		defer svr.Close()

		resp, err := http.Get(svr.URL + path)
		Ω(err).To(Succeed())
		Ω(resp.StatusCode).To(Equal(http.StatusBadRequest))

		resp, err = http.Get(svr.URL + path + "?name=a&namespace=b")
		Ω(err).To(Succeed())
		Ω(resp.StatusCode).To(Equal(http.StatusOK))

		ev := <-ch
		Ω(ev.Object.GetName()).To(Equal("a"))
		Ω(ev.Object.GetNamespace()).To(Equal("b"))
	})

})
