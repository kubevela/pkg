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

package runtime_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"cuelang.org/go/cue/build"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kubevela/pkg/cue/cuex/providers/base64"
	"github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/slices"
	"github.com/kubevela/pkg/util/test/bootstrap"
)

func TestPackageManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Run package manager test")
}

var _ = bootstrap.InitKubeBuilderForTest(bootstrap.WithCRDPath("../../../crds/cue.oam.dev_packages.yaml"))

var _ = Describe("Test Cuex Runtime", func() {

	It("Test PackageManager Run", func() {
		pm := runtime.NewPackageManager(
			runtime.WithResyncPeriod(time.Minute),
			runtime.WithInternalPackage{Package: base64.Package})
		ctx := context.Background()
		cli := singleton.KubeClient.Get()
		server := newTestServer()
		defer server.Close()
		_pkg, _ := newTestPackage(server.URL)
		_pkg.SetNamespace("default")
		Ω(cli.Create(ctx, _pkg)).To(Succeed())

		Ω(pm.LoadExternalPackages(ctx)).To(Succeed())
		stopCh := make(chan struct{})
		go pm.ListenExternalPackages(stopCh)
		defer close(stopCh)
		Ω(len(pm.GetPackages())).To(Equal(2))
		Ω(len(pm.GetImports())).To(Equal(2))
		Ω(pm.GetProviders()["base64"]).ToNot(BeNil())
		Ω(pm.GetProviders()["ext-test"]).ToNot(BeNil())

		By("Test update package")
		Ω(cli.Get(ctx, types.NamespacedName{Name: _pkg.Name, Namespace: _pkg.Namespace}, _pkg)).To(Succeed())
		_pkg.Spec.Path = "remote/ex/test"
		Ω(cli.Update(ctx, _pkg)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Ω(slices.Index(pm.GetImports(), func(i *build.Instance) bool { return i.ImportPath == "remote/ex/test" }) >= 0).To(BeTrue())
		}).WithTimeout(5 * time.Second).Should(Succeed())

		Ω(cli.Get(ctx, types.NamespacedName{Name: _pkg.Name, Namespace: _pkg.Namespace}, _pkg)).To(Succeed())
		Ω(cli.Delete(ctx, _pkg)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Ω(kerrors.IsNotFound(cli.Get(ctx, types.NamespacedName{Name: _pkg.Name, Namespace: _pkg.Namespace}, _pkg))).To(BeTrue())
			g.Ω(len(pm.GetPackages())).To(Equal(1))
		}).WithTimeout(5 * time.Second).Should(Succeed())
	})

})
