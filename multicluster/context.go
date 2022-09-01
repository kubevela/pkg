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

package multicluster

import "context"

type key int

const (
	// clusterKey is the context key for multi-cluster request
	clusterKey key = iota
)

// WithCluster returns a copy of parent in which the cluster value is set
func WithCluster(parent context.Context, cluster string) context.Context {
	return context.WithValue(parent, clusterKey, cluster)
}

// ClusterFrom returns the value of the cluster key on the ctx
func ClusterFrom(ctx context.Context) (string, bool) {
	cluster, ok := ctx.Value(clusterKey).(string)
	return cluster, ok
}
