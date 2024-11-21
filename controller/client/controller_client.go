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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/multicluster"
)

var (
	// CachedGVKs identifies the GVKs of resources to be cached during dispatching
	CachedGVKs = ""
)

var _ client.NewClientFunc = DefaultNewControllerClient

// DefaultNewControllerClient function for creating controller client
func DefaultNewControllerClient(config *rest.Config, options client.Options) (c client.Client, err error) {
	rawClient, err := multicluster.NewDefaultClient(config, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw client: %w", err)
	}
	rawClient = WrapDefaultTimeoutClient(rawClient)

	mClient := &monitorClient{rawClient}
	mCache := &monitorCache{options.Cache.Reader}

	uncachedStructuredGVKs := map[schema.GroupVersionKind]struct{}{}
	for _, obj := range options.Cache.DisableFor {
		gvk, err := mClient.GroupVersionKindFor(obj)
		if err != nil {
			return nil, err
		}
		uncachedStructuredGVKs[gvk] = struct{}{}
	}

	cachedUnstructuredGVKs := map[schema.GroupVersionKind]struct{}{}
	for _, s := range strings.Split(CachedGVKs, ",") {
		s = strings.Trim(s, " ")
		if len(s) > 0 {
			gvk, _ := schema.ParseKindArg(s)
			if gvk == nil {
				return nil, fmt.Errorf("invalid cached gvk: %s", s)
			}
			cachedUnstructuredGVKs[*gvk] = struct{}{}
		}
	}

	dClient := &delegatingClient{
		scheme: mClient.Scheme(),
		mapper: mClient.RESTMapper(),
		client: mClient,
		Reader: &delegatingReader{
			CacheReader:            mCache,
			ClientReader:           mClient,
			scheme:                 mClient.Scheme(),
			uncachedStructuredGVKs: uncachedStructuredGVKs,
			cachedUnstructuredGVKs: cachedUnstructuredGVKs,
		},
		Writer:                       mClient,
		StatusClient:                 mClient,
		SubResourceClientConstructor: mClient,
	}

	return dClient, nil
}
