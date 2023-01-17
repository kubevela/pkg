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

package sharding_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/test/bootstrap"
)

func TestSharding(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Run sharding test")
}

var _ = bootstrap.InitKubeBuilderForTest()

var _ = Describe("Test sharding", func() {

	It("Test sharding cache", func() {
		cfg := singleton.KubeConfig.Get()
		cli := singleton.KubeClient.Get()
		fs := pflag.NewFlagSet("-", pflag.PanicOnError)
		sharding.AddFlags(fs)
		Ω(fs.Parse([]string{"--shard-id=test", "--enable-sharding"})).To(Succeed())

		fn := sharding.WrapNewCacheFunc(cache.New, &corev1.ConfigMap{})
		store, err := fn(cfg, cache.Options{})
		Ω(err).To(Succeed())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = store.Start(ctx) }()
		Ω(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: "x", Namespace: metav1.NamespaceDefault, Labels: map[string]string{sharding.LabelKubeVelaShardID: "test"},
		}})).To(Succeed())
		Ω(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: "y", Namespace: metav1.NamespaceDefault,
		}})).To(Succeed())
		Ω(cli.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: "y", Namespace: metav1.NamespaceDefault,
		}})).To(Succeed())
		Eventually(func(g Gomega) {
			cms := &corev1.ConfigMapList{}
			Ω(store.List(ctx, cms)).Should(Succeed())
			Ω(len(cms.Items)).Should(Equal(1))
			Ω(cms.Items[0].Name).Should(Equal("x"))
			Ω(store.Get(ctx, types.NamespacedName{Namespace: metav1.NamespaceDefault, Name: "y"}, &corev1.Secret{})).Should(Succeed())
		}).WithTimeout(5 * time.Second).Should(Succeed())
	})

})
