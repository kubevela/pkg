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

package multicluster_test

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"k8s.io/client-go/rest"
	controllerruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/jsonutil"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/rand"
	"github.com/kubevela/pkg/util/singleton"
)

var _ = Describe("Test remote multicluster client", func() {

	It("Test client", func() {
		cfg := singleton.KubeConfig.Get()
		cert, err := tls.X509KeyPair(cfg.CertData, cfg.KeyData)
		Ω(err).To(Succeed())

		badCfg := rest.CopyConfig(cfg)
		badCfg.Host = ""
		_, err = multicluster.NewRemoteClusterClient(badCfg, controllerruntimeclient.Options{})
		Ω(err).NotTo(Succeed())

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/bad-cluster/") {
				http.Error(w, "bad cluster", 400)
				return
			}
			const prefix = "/apis/cluster.core.oam.dev/v1alpha1/clustergateways/managed/proxy"
			req := r.Clone(r.Context())
			h, _ := url.Parse(cfg.Host)
			h.Path = strings.TrimPrefix(r.URL.Path, prefix)
			req.URL = h
			req.RequestURI = ""

			cli := &http.Client{Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					Certificates:       []tls.Certificate{cert},
				}}}
			resp, err := cli.Do(req)
			if err != nil {
				http.Error(w, err.Error(), resp.StatusCode)
				return
			}
			for key, values := range resp.Header {
				for _, val := range values {
					w.Header().Add(key, val)
				}
			}
			w.WriteHeader(resp.StatusCode)
			if resp.Body != nil {
				defer resp.Body.Close()
				_, _ = io.Copy(w, resp.Body)
			}
		})

		l, err := net.Listen("tcp", "127.0.0.1:0")
		Ω(err).To(Succeed())
		server := &httptest.Server{
			Listener: l,
			Config: &http.Server{
				Handler:  handler,
				ErrorLog: log.New(io.Discard, "httptest", log.Ltime),
			},
		}
		server.StartTLS()

		defer server.Close()
		copied := rest.CopyConfig(cfg)
		copied.Host = server.URL
		copied.CAData = nil
		copied.Insecure = true

		c, err := multicluster.NewRemoteClusterClient(copied, controllerruntimeclient.Options{})
		Ω(err).To(Succeed())

		By("Test unstructured remote client")
		ctx := multicluster.WithCluster(context.Background(), "managed")
		namespace, name := "test-"+rand.RandomString(4), "fake"
		Ω(k8s.EnsureNamespace(ctx, c, namespace)).To(Succeed())
		_deploy := &appsv1.Deployment{
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
		o, err := jsonutil.AsType[map[string]interface{}](_deploy)
		Ω(err).To(Succeed())
		deploy := &unstructured.Unstructured{Object: *o}
		deploy.SetAPIVersion("apps/v1")
		deploy.SetKind("Deployment")
		Ω(c.Create(ctx, deploy)).To(Succeed())
		Ω(c.Get(ctx, controllerruntimeclient.ObjectKey{Namespace: namespace, Name: name}, deploy)).To(Succeed())
		svc := &unstructured.UnstructuredList{}
		svc.SetAPIVersion("v1")
		svc.SetKind("ServiceList")
		Ω(c.List(ctx, svc)).To(Succeed())
		Ω(c.Update(ctx, deploy)).To(Succeed())
		Ω(c.Patch(ctx, deploy, controllerruntimeclient.Merge)).To(Succeed())
		Ω(c.Status().Update(ctx, deploy)).To(Succeed())
		Ω(c.Status().Patch(ctx, deploy, controllerruntimeclient.Merge)).To(Succeed())
		updateBody := deploy.DeepCopy()
		updateBody.SetName("")
		updateBody.SetNamespace("")
		Ω(c.SubResource("status").Get(ctx, deploy, updateBody.DeepCopy())).To(Succeed())
		Ω(c.SubResource("status").Update(ctx, deploy, &controllerruntimeclient.SubResourceUpdateOptions{SubResourceBody: updateBody})).To(Succeed())
		Ω(c.SubResource("status").Patch(ctx, deploy, controllerruntimeclient.Merge, &controllerruntimeclient.SubResourcePatchOptions{SubResourceBody: updateBody})).To(Succeed())
		_sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
		_sa.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ServiceAccount"))
		sa, err := jsonutil.AsType[unstructured.Unstructured](_sa)
		Ω(err).To(Succeed())
		Ω(controllerruntimeclient.IgnoreAlreadyExists(c.Create(ctx, _sa))).To(Succeed())
		_tr := authnv1.TokenRequest{}
		_tr.SetGroupVersionKind(authnv1.SchemeGroupVersion.WithKind("TokenRequest"))
		tr, err := jsonutil.AsType[unstructured.Unstructured](_tr)
		Ω(err).To(Succeed())
		Ω(c.SubResource("token").Create(ctx, sa, tr)).To(Succeed())
		Ω(c.Delete(ctx, deploy)).To(Succeed())
		cm := &unstructured.Unstructured{}
		cm.SetAPIVersion("v1")
		cm.SetKind("ConfigMap")
		Ω(c.DeleteAllOf(ctx, cm, controllerruntimeclient.InNamespace(namespace))).To(Succeed())
		Ω(c.Scheme().IsGroupRegistered("apps")).To(BeTrue())
		_, err = c.RESTMapper().ResourceSingularizer("configmaps")
		Ω(err).To(Succeed())
		Ω(k8s.ClearNamespace(ctx, c, namespace)).To(Succeed())

		namespace, name = "test-"+rand.RandomString(4), "fake"
		Ω(k8s.EnsureNamespace(ctx, c, namespace)).To(Succeed())
		deploy2 := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec: appsv1.DeploymentSpec{
				Replicas: pointer.Int32(1),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test2"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test2"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "test"}}},
				},
			},
		}
		Ω(c.Create(ctx, deploy2)).To(Succeed())
		Ω(c.Get(ctx, controllerruntimeclient.ObjectKey{Namespace: namespace, Name: name}, deploy2)).To(Succeed())
		Ω(c.Update(ctx, deploy2)).To(Succeed())
		Ω(c.Patch(ctx, deploy2, controllerruntimeclient.Merge)).To(Succeed())
		Ω(c.Status().Update(ctx, deploy2)).To(Succeed())
		Ω(c.Status().Patch(ctx, deploy2, controllerruntimeclient.Merge)).To(Succeed())
		updateBody2 := deploy2.DeepCopy()
		updateBody2.SetName("")
		updateBody2.SetNamespace("")
		Ω(c.SubResource("status").Get(ctx, deploy2, updateBody2.DeepCopy())).To(Succeed())
		Ω(c.SubResource("status").Update(ctx, deploy2, &controllerruntimeclient.SubResourceUpdateOptions{SubResourceBody: updateBody2})).To(Succeed())
		Ω(c.SubResource("status").Patch(ctx, deploy2, controllerruntimeclient.Merge, &controllerruntimeclient.SubResourcePatchOptions{SubResourceBody: updateBody2})).To(Succeed())
		Ω(c.Delete(ctx, deploy2)).To(Succeed())
		Ω(k8s.ClearNamespace(ctx, c, namespace)).To(Succeed())

		By("Test bad resource")
		ar := &unstructured.Unstructured{}
		ar.SetAPIVersion("xxx")
		Ω(c.Create(ctx, ar)).NotTo(Succeed())
		Ω(c.Update(ctx, ar)).NotTo(Succeed())
		Ω(c.Patch(ctx, ar, controllerruntimeclient.Merge)).NotTo(Succeed())
		Ω(c.Get(ctx, types.NamespacedName{}, ar)).NotTo(Succeed())
		Ω(c.Delete(ctx, ar)).NotTo(Succeed())
		Ω(c.DeleteAllOf(ctx, ar)).NotTo(Succeed())

		cms := &unstructured.UnstructuredList{}
		cms.SetAPIVersion("v1")
		cms.SetKind("ConfigMapList")
		Ω(c.List(multicluster.WithCluster(context.Background(), "bad-cluster"), cms)).NotTo(Succeed())
	})

})

func TestParamCodec(t *testing.T) {
	paramCodec := multicluster.NewNoConversionParamCodec()
	_, err := paramCodec.EncodeParameters(&unstructured.Unstructured{}, schema.GroupVersion{})
	require.NoError(t, err)
	err = paramCodec.DecodeParameters(url.Values{}, schema.GroupVersion{}, nil)
	require.Error(t, err)
}
