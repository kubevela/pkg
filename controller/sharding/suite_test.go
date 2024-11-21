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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/kubevela/pkg/meta"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/singleton"

	"github.com/kubevela/pkg/util/test/bootstrap"
)

func TestSharding(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Run client package test")
}

var _ = bootstrap.InitKubeBuilderForTest()

var _ = Describe("Test sharding", func() {

	It("Test static scheduler", func() {
		fs := pflag.NewFlagSet("-", pflag.ExitOnError)
		sharding.AddFlags(fs)
		Ω(fs.Parse([]string{"--enable-sharding", "--shard-id=s", "--schedulable-shards=s,t"})).To(Succeed())
		Ω(sharding.SchedulableShards).To(Equal([]string{"s", "t"}))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cfg, cli := singleton.KubeConfig.Get(), singleton.KubeClient.Get()
		sharding.DefaultScheduler.Reload()
		_ = sharding.DefaultScheduler.Get()

		By("Test static scheduler")
		scheduler := sharding.NewStaticScheduler([]string{"s"})
		go scheduler.Start(ctx)
		cm1 := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "scheduled", Namespace: metav1.NamespaceDefault}}
		Ω(scheduler.Schedule(cm1)).To(BeTrue())
		Ω(cli.Create(ctx, cm1)).To(Succeed())
		scheduler = sharding.NewStaticScheduler([]string{""})
		go scheduler.Start(ctx)
		cm2 := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "unscheduled", Namespace: metav1.NamespaceDefault}}
		Ω(scheduler.Schedule(cm1)).To(BeFalse())
		Ω(cli.Create(ctx, cm2)).To(Succeed())

		By("Test cache")
		store, err := sharding.BuildCache(&corev1.ConfigMap{})(cfg, cache.Options{Scheme: scheme.Scheme})
		Ω(err).To(Succeed())
		go func() { _ = store.Start(ctx) }()
		Eventually(func(g Gomega) {
			cms := &corev1.ConfigMapList{}
			g.Expect(store.List(ctx, cms)).To(Succeed())
			g.Expect(len(cms.Items)).To(Equal(1))
			g.Expect(cms.Items[0].Name).To(Equal("scheduled"))
			g.Expect(kerrors.IsNotFound(store.Get(ctx, types.NamespacedName{Name: cm2.Name, Namespace: cm2.Namespace}, &corev1.ConfigMap{}))).To(BeTrue())
		}).WithTimeout(5 * time.Second).Should(Succeed())
	})

	It("Test dynamic scheduler", func() {
		fs := pflag.NewFlagSet("-", pflag.ExitOnError)
		sharding.AddFlags(fs)
		Ω(fs.Parse([]string{"--enable-sharding", "--schedulable-shards="})).To(Succeed())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cli := singleton.KubeClient.Get()
		sharding.DefaultScheduler.Reload()
		scheduler := sharding.DefaultScheduler.Get()
		go scheduler.Start(ctx)
		Ω(k8s.EnsureNamespace(ctx, cli, meta.NamespaceVelaSystem)).To(Succeed())

		o := &unstructured.Unstructured{Object: map[string]interface{}{}}
		Ω(scheduler.Schedule(o.DeepCopy())).To(BeFalse())

		newPod := func(name string) *corev1.Pod {
			return &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: meta.NamespaceVelaSystem, Name: name,
					Labels: map[string]string{
						sharding.LabelKubeVelaShardID: name,
						"app.kubernetes.io/name":      meta.Name,
					},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  name,
					Image: "busybox",
				}}},
			}
		}
		setPodHealthy := func(pod *corev1.Pod) {
			pod.Status = corev1.PodStatus{Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}}}
		}

		s1 := newPod("s1")
		Ω(cli.Create(ctx, s1)).To(Succeed())
		setPodHealthy(s1)
		Ω(cli.Status().Update(ctx, s1)).To(Succeed())
		Eventually(func(g Gomega) {
			_o := o.DeepCopy()
			g.Ω(scheduler.Schedule(_o)).To(BeTrue())
			g.Ω(_o.GetLabels()[sharding.LabelKubeVelaScheduledShardID]).To(Equal("s1"))
		}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())

		s2 := newPod("s2")
		Ω(cli.Create(ctx, s2)).To(Succeed())
		setPodHealthy(s2)
		Ω(cli.Status().Update(ctx, s2)).To(Succeed())

		Ω(cli.Delete(ctx, s1)).To(Succeed())
		Eventually(func(g Gomega) {
			_o := o.DeepCopy()
			g.Ω(scheduler.Schedule(_o)).To(BeTrue())
			g.Ω(_o.GetLabels()[sharding.LabelKubeVelaScheduledShardID]).To(Equal("s2"))
		}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())

		_ = k8s.AddLabel(s2, sharding.LabelKubeVelaShardID, "s0")
		Ω(cli.Update(ctx, s2)).To(Succeed())
		s2Key := types.NamespacedName{Namespace: meta.NamespaceVelaSystem, Name: "s2"}
		Eventually(func(g Gomega) {
			g.Ω(cli.Get(ctx, s2Key, s2)).To(Succeed())
			g.Ω(s2.GetLabels()[sharding.LabelKubeVelaShardID]).Should(Equal("s0"))
			_o := o.DeepCopy()
			g.Ω(scheduler.Schedule(_o)).To(BeTrue())
			g.Ω(_o.GetLabels()[sharding.LabelKubeVelaScheduledShardID]).To(Equal("s0"))
		}).WithTimeout(5 * time.Second).WithPolling(time.Second).Should(Succeed())
	})

})
