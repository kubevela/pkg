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

package singleton

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// UserAgent .
var UserAgent = NewSingleton[string](nil)

// KubeConfig .
var KubeConfig = NewSingleton[*rest.Config](func() *rest.Config {
	cfg := config.GetConfigOrDie()
	cfg.UserAgent = UserAgent.Get()
	return cfg
})

// RESTMapper .
var RESTMapper = NewSingletonE[meta.RESTMapper](func() (meta.RESTMapper, error) {
	httpClient, err := rest.HTTPClientFor(KubeConfig.Get())
	if err != nil {
		return nil, err
	}
	return apiutil.NewDynamicRESTMapper(KubeConfig.Get(), httpClient)
})

// KubeClient .
var KubeClient = NewSingletonE[client.Client](func() (client.Client, error) {
	return client.New(KubeConfig.Get(), client.Options{
		Scheme: scheme.Scheme,
		Mapper: RESTMapper.Get(),
	})
})

// StaticClient .
var StaticClient = NewSingletonE[kubernetes.Interface](func() (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(KubeConfig.Get())
})

// DynamicClient .
var DynamicClient = NewSingletonE[dynamic.Interface](func() (dynamic.Interface, error) {
	return dynamic.NewForConfig(KubeConfig.Get())
})

// ReloadClients should be called when KubeConfig is called to update related clients
func ReloadClients() {
	RESTMapper.Reload()
	KubeClient.Reload()
	StaticClient.Reload()
	DynamicClient.Reload()
}
