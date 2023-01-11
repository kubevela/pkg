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

package kube

import (
	"context"

	_ "embed"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"
)

type GetVar struct {
	Cluster string                     `json:"cluster"`
	Value   *unstructured.Unstructured `json:"value"`
}

func Get(ctx context.Context, obj *GetVar) (*GetVar, error) {
	ctx = multicluster.WithCluster(ctx, obj.Cluster)
	return obj, singleton.KubeClient.Get().Get(ctx, client.ObjectKeyFromObject(obj.Value), obj.Value)
}

type ListFilter struct {
	Namespace      string            `json:"namespace,omitempty"`
	MatchingLabels map[string]string `json:"matchingLabels,omitempty"`
}

type ListVar struct {
	Cluster string                         `json:"cluster"`
	Filter  *ListFilter                    `json:"filter,omitempty"`
	List    *unstructured.UnstructuredList `json:"list"`
}

func List(ctx context.Context, objs *ListVar) (*ListVar, error) {
	ctx = multicluster.WithCluster(ctx, objs.Cluster)
	var listOpts []client.ListOption
	if objs.Filter != nil && objs.Filter.Namespace != "" {
		listOpts = append(listOpts, client.InNamespace(objs.Filter.Namespace))
	}
	if objs.Filter != nil && objs.Filter.MatchingLabels != nil {
		listOpts = append(listOpts, client.MatchingLabels(objs.Filter.MatchingLabels))
	}
	return objs, singleton.KubeClient.Get().List(ctx, objs.List, listOpts...)
}

const ProviderName = "kube"

//go:embed kube.cue
var template string

var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"get":  cuexruntime.GenericProviderFn[GetVar, GetVar](Get),
	"list": cuexruntime.GenericProviderFn[ListVar, ListVar](List),
}))
