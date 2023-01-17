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

package sharding

import (
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WrapNewCacheFunc add shard-id label selector for given typed object
func WrapNewCacheFunc(fn cache.NewCacheFunc, shardingObjects ...client.Object) cache.NewCacheFunc {
	return func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		if EnableSharding {
			if opts.SelectorsByObject == nil {
				opts.SelectorsByObject = cache.SelectorsByObject{}
			}
			ls := labels.SelectorFromSet(map[string]string{LabelKubeVelaShardID: ShardID})
			for _, obj := range shardingObjects {
				opts.SelectorsByObject[obj] = cache.ObjectSelector{Label: ls}
			}
		}
		return fn(config, opts)
	}
}
