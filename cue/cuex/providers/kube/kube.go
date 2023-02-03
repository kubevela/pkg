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
	"fmt"

	_ "embed"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/k8s/patch"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"
)

const (
	// AnnoLastAppliedConfigSuffix is the suffix for last applied config
	AnnoLastAppliedConfigSuffix = "oam.dev/last-applied-configuration"
	// AnnoLastAppliedTimeSuffix is suffix for last applied time
	AnnoLastAppliedTimeSuffix = "oam.dev/last-applied-time"
)

// ResourceVars .
type ResourceVars struct {
	Cluster  string                     `json:"cluster"`
	Resource *unstructured.Unstructured `json:"resource"`
	Options  ApplyOptions               `json:"options"`
}

// ApplyOptions .
type ApplyOptions struct {
	ThreeWayMergePatch ThreeWayMergePatchOptions `json:"threeWayMergePatch"`
}

// ThreeWayMergePatchOptions .
type ThreeWayMergePatchOptions struct {
	Enabled          bool   `json:"enabled"`
	AnnotationPrefix string `json:"annotationPrefix"`
}

// ResourceParams is the params for resource
type ResourceParams providers.Params[ResourceVars]

// ResourceReturns is the returns for resource
type ResourceReturns providers.Returns[*unstructured.Unstructured]

// Apply .
func Apply(ctx context.Context, getParams *ResourceParams) (*ResourceReturns, error) {
	params := getParams.Params
	ctx = multicluster.WithCluster(ctx, params.Cluster)
	workload := params.Resource
	cli := singleton.KubeClient.Get()
	existing := &unstructured.Unstructured{}
	existing.GetObjectKind().SetGroupVersionKind(workload.GetObjectKind().GroupVersionKind())

	if err := cli.Get(ctx, client.ObjectKeyFromObject(workload), existing); err != nil {
		if errors.IsNotFound(err) {
			if params.Options.ThreeWayMergePatch.Enabled {
				b, err := workload.MarshalJSON()
				if err != nil {
					return nil, err
				}
				annoKey := fmt.Sprintf("%s.%s", params.Options.ThreeWayMergePatch.AnnotationPrefix, AnnoLastAppliedConfigSuffix)
				if err := k8s.AddAnnotation(workload, annoKey, string(b)); err != nil {
					return nil, err
				}
			}
			if err := cli.Create(ctx, workload); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		patcher, err := patch.ThreeWayMergePatch(existing, workload, &patch.PatchAction{
			UpdateAnno:            params.Options.ThreeWayMergePatch.Enabled,
			AnnoLastAppliedConfig: fmt.Sprintf("%s.%s", params.Options.ThreeWayMergePatch.AnnotationPrefix, AnnoLastAppliedConfigSuffix),
			AnnoLastAppliedTime:   fmt.Sprintf("%s.%s", params.Options.ThreeWayMergePatch.AnnotationPrefix, AnnoLastAppliedTimeSuffix),
		})
		if err != nil {
			return nil, err
		}
		if err := cli.Patch(ctx, workload, patcher); err != nil {
			return nil, err
		}
	}
	return &ResourceReturns{Returns: workload}, nil
}

// Get .
func Get(ctx context.Context, getParams *ResourceParams) (*ResourceReturns, error) {
	params := getParams.Params
	ctx = multicluster.WithCluster(ctx, params.Cluster)
	if err := singleton.KubeClient.Get().Get(ctx, client.ObjectKeyFromObject(params.Resource), params.Resource); err != nil {
		return nil, err
	}
	return &ResourceReturns{Returns: params.Resource}, nil
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

// Patch patches a kubernetes resource with patch strategy
func Patch(ctx context.Context, patchParams *PatchParams) (*ResourceReturns, error) {
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
	case "strategic":
		patchType = types.StrategicMergePatchType
	case "json":
		patchType = types.JSONPatchType
	default:
		patchType = types.MergePatchType
	}
	if err := singleton.KubeClient.Get().Patch(ctx, params.Resource, client.RawPatch(patchType, patchData)); err != nil {
		return nil, err
	}
	return &ResourceReturns{Returns: params.Resource}, nil
}

// ProviderName .
const ProviderName = "kube"

//go:embed kube.cue
var template string

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"apply": cuexruntime.GenericProviderFn[ResourceParams, ResourceReturns](Apply),
	"get":   cuexruntime.GenericProviderFn[ResourceParams, ResourceReturns](Get),
	"list":  cuexruntime.GenericProviderFn[ListParams, ListReturns](List),
	"patch": cuexruntime.GenericProviderFn[PatchParams, ResourceReturns](Patch),
}))
