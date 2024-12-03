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

package client

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/k8s"
)

// delegatingClient delegate the requests to the cacheReader if the target
// resource is cached.
type delegatingClient struct {
	client.Reader
	client.Writer
	client.StatusClient
	client.SubResourceClientConstructor

	scheme *runtime.Scheme
	mapper meta.RESTMapper
	client client.Client
}

// Scheme returns the scheme this client is using.
func (d *delegatingClient) Scheme() *runtime.Scheme {
	return d.scheme
}

// RESTMapper returns the rest mapper this client is using.
func (d *delegatingClient) RESTMapper() meta.RESTMapper {
	return d.mapper
}

// RESTMapper returns the rest mapper this client is using.
func (d *delegatingClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return d.client.GroupVersionKindFor(obj)
}

// RESTMapper returns the rest mapper this client is using.
func (d *delegatingClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return d.client.IsObjectNamespaced(obj)
}

// delegatingReader extend the delegatingReader from controller-runtime/pkg/client
// 1. for requests not in local cluster, disable cache
// 2. for structured types, inherit the cache blacklist
// 3. for unstructured types, use cache whitelist
type delegatingReader struct {
	CacheReader  client.Reader
	ClientReader client.Reader

	uncachedStructuredGVKs map[schema.GroupVersionKind]struct{}
	cachedUnstructuredGVKs map[schema.GroupVersionKind]struct{}
	scheme                 *runtime.Scheme
}

var _ client.Reader = &delegatingReader{}

func (d *delegatingReader) shouldBypassCache(obj runtime.Object) (bool, error) {
	gvk, err := apiutil.GVKForObject(obj, d.scheme)
	if err != nil {
		return false, err
	}
	if meta.IsListType(obj) {
		gvk.Kind = strings.TrimSuffix(gvk.Kind, "List")
	}
	if k8s.IsUnstructuredObject(obj) {
		_, shouldCache := d.cachedUnstructuredGVKs[gvk]
		return !shouldCache, nil
	}
	_, shouldNotCache := d.uncachedStructuredGVKs[gvk]
	return shouldNotCache, nil
}

// Get retrieves an obj for a given object key from the Kubernetes Cluster.
func (d *delegatingReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if isUncached, err := d.shouldBypassCache(obj); err != nil {
		return err
	} else if cluster, _ := multicluster.ClusterFrom(ctx); !multicluster.IsLocal(cluster) || isUncached {
		return d.ClientReader.Get(ctx, key, obj, opts...)
	}
	return d.CacheReader.Get(ctx, key, obj, opts...)
}

// List retrieves list of objects for a given namespace and list options.
func (d *delegatingReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if isUncached, err := d.shouldBypassCache(list); err != nil {
		return err
	} else if cluster, _ := multicluster.ClusterFrom(ctx); !multicluster.IsLocal(cluster) || isUncached {
		return d.ClientReader.List(ctx, list, opts...)
	}
	return d.CacheReader.List(ctx, list, opts...)
}
