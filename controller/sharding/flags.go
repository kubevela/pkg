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

import "github.com/spf13/pflag"

// ShardID the id for sharding
var ShardID string

// EnableSharding whether enable sharding
var EnableSharding bool

// AddFlags add sharding flags
func AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&EnableSharding, "enable-sharding", EnableSharding, "Whether to enable sharding.")
	fs.StringVar(&ShardID, "shard-id", ShardID, "The id for sharding. If empty, no sharding.")
}
