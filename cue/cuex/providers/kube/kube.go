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
	"encoding/json"

	_ "embed"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"
)

// GetVars .
type GetVars struct {
	Cluster  string                     `json:"cluster"`
	Resource *unstructured.Unstructured `json:"resource"`
}

// GetParams is the params for get
type GetParams providers.Params[GetVars]

// GetReturns is the returns for get
type GetReturns providers.Returns[*unstructured.Unstructured]

// Get .
func Get(ctx context.Context, getParams *GetParams) (*GetReturns, error) {
	params := getParams.Params
	ctx = multicluster.WithCluster(ctx, params.Cluster)
	if err := singleton.KubeClient.Get().Get(ctx, client.ObjectKeyFromObject(params.Resource), params.Resource); err != nil {
		return nil, err
	}
	return &GetReturns{Returns: params.Resource}, nil
}

// ListFilter filter for list resources
type ListFilter struct {
	Namespace      string            `json:"namespace,omitempty"`
	MatchingLabels map[string]string `json:"matchingLabels,omitempty"`
}

// ListVars is the vars for list
type ListVars struct {
	Cluster  string                     `json:"cluster"`
	Filter   *ListFilter                `json:"filter,omitempty"`
	Resource *unstructured.Unstructured `json:"resource"`
}

// ListParams is the params for list
type ListParams providers.Params[ListVars]

// ListReturns is the returns for list
type ListReturns providers.Returns[*unstructured.UnstructuredList]

// List .
func List(ctx context.Context, listParams *ListParams) (*ListReturns, error) {
	params := listParams.Params
	ctx = multicluster.WithCluster(ctx, params.Cluster)
	var listOpts []client.ListOption
	if params.Filter != nil && params.Filter.Namespace != "" {
		listOpts = append(listOpts, client.InNamespace(params.Filter.Namespace))
	}
	if params.Filter != nil && params.Filter.MatchingLabels != nil {
		listOpts = append(listOpts, client.MatchingLabels(params.Filter.MatchingLabels))
	}
	returns := &ListReturns{
		Returns: &unstructured.UnstructuredList{Object: params.Resource.Object},
	}
	if err := singleton.KubeClient.Get().List(ctx, returns.Returns, listOpts...); err != nil {
		return returns, err
	}
	return returns, nil
}

// PatchVars is the vars for patch
type PatchVars struct {
	Cluster  string                     `json:"cluster"`
	Resource *unstructured.Unstructured `json:"resource"`
	Patch    Patcher                    `json:"patch"`
}

// Patcher is the patcher
type Patcher struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// PatchParams is the params for patch
type PatchParams providers.Params[PatchVars]

// PatchReturns is the returns for patch
type PatchReturns providers.Returns[*unstructured.Unstructured]

// Patch patches a kubernetes resource with patch strategy
func Patch(ctx context.Context, patchParams *PatchParams) (*PatchReturns, error) {
	params := patchParams.Params
	ctx = multicluster.WithCluster(ctx, params.Cluster)
	err := singleton.KubeClient.Get().Get(ctx, client.ObjectKeyFromObject(params.Resource), params.Resource)
	if err != nil {
		return nil, err
	}
	patchData, err := json.Marshal(params.Patch.Data)
	if err != nil {
		return nil, err
	}
	var patchType types.PatchType
	switch params.Patch.Type {
	case "merge":
		patchType = types.MergePatchType
	case "json":
		patchType = types.JSONPatchType
	default:
		patchType = types.StrategicMergePatchType
	}
	if err := singleton.KubeClient.Get().Patch(ctx, params.Resource, client.RawPatch(patchType, patchData)); err != nil {
		return nil, err
	}
	return &PatchReturns{Returns: params.Resource}, nil
}

// ProviderName .
const ProviderName = "kube"

//go:embed kube.cue
var template string

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"get":   cuexruntime.GenericProviderFn[GetParams, GetReturns](Get),
	"list":  cuexruntime.GenericProviderFn[ListParams, ListReturns](List),
	"patch": cuexruntime.GenericProviderFn[PatchParams, PatchReturns](Patch),
}))
