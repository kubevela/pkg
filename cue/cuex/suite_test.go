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

package cuex_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubevela/pkg/apis/cue/v1alpha1"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/test/bootstrap"
)

func TestCuex(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Run Cuex test")
}

var _ = bootstrap.InitKubeBuilderForTest(bootstrap.WithCRDPath("../../crds/cue.oam.dev_packages.yaml"))

type toUpperIn struct {
	Input string `json:"input"`
}

type toUpperOut struct {
	Output string `json:"output"`
}

var _ = Describe("Test Cuex Compiler", func() {
	It("Test Cuex run", func() {
		ctx := context.Background()
		cli := singleton.KubeClient.Get()

		By("Setting up external provider server")
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			bs, err := io.ReadAll(request.Body)
			if err != nil {
				writer.WriteHeader(400)
				return
			}
			in := &toUpperIn{}
			if err := json.Unmarshal(bs, in); err != nil {
				writer.WriteHeader(400)
				return
			}
			out := &toUpperOut{Output: strings.ToUpper(in.Input)}
			if bs, err = json.Marshal(out); err != nil {
				writer.WriteHeader(500)
				return
			}
			writer.WriteHeader(200)
			_, _ = writer.Write(bs)
		}))
		defer server.Close()

		pkg := &v1alpha1.Package{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "string-util"},
			Spec: v1alpha1.PackageSpec{
				Path: "ext/string-util",
				Provider: &v1alpha1.Provider{
					Protocol: v1alpha1.ProtocolHTTP,
					Endpoint: server.URL,
				},
				Templates: map[string]string{
					"scheme.cue": `
					package stringutil
					#ToUpper: {
						#do: "toUpper"
						#provider: "string-util"
						$params: input: string
						$returns?: output: string
					}
				`,
				},
			},
		}
		Ω(cli.Create(ctx, pkg)).To(Succeed())

		By("Create example secret")
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test"}}
		secret.StringData = map[string]string{"key": "value"}
		Ω(cli.Create(ctx, secret)).To(Succeed())

		v, err := cuex.CompileString(ctx, `
		import (
			"vela/base64"
			"vela/kube"
			sutil "ext/string-util"
		)

		secret: kube.#Get & {
			$params: {
				resource: {
					apiVersion: "v1"
					kind: "Secret"
					metadata: name: "test"
					metadata: namespace: "default"
				}
			}
		}

		decode: base64.#Decode & {
			$params: secret.$returns.data["key"]
		}

		toUpper: sutil.#ToUpper & {
			$params: input: decode.$returns
		}
		
		output: toUpper.$returns.output
	`)
		Ω(err).To(Succeed())
		s, err := v.LookupPath(cue.ParsePath("output")).String()
		Ω(err).To(Succeed())
		Ω(s).To(Equal("VALUE"))
	})
})
