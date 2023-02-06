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
	"time"

	"k8s.io/klog/v2"

	"github.com/kubevela/pkg/meta"
	"github.com/kubevela/pkg/util/singleton"
	velaslices "github.com/kubevela/pkg/util/slices"
)

const (
	// LabelKubeVelaShardID label key for identify the shard id for the controller
	LabelKubeVelaShardID = "controller.core.oam.dev/shard-id"
	// LabelKubeVelaScheduledShardID label key for identify the scheduled shard id for the resource
	LabelKubeVelaScheduledShardID = "controller.core.oam.dev/scheduled-shard-id"
	// MasterShardID the master shard id
	MasterShardID = "master"
)

var (
	// ShardID the id of the shard
	ShardID = MasterShardID
	// EnableSharding whether enable sharding
	EnableSharding bool
	// SchedulableShards the shards for schedule
	SchedulableShards []string
	// DynamicDiscoverySchedulerResyncPeriod resync period for default dynamic discovery scheduler
	DynamicDiscoverySchedulerResyncPeriod = 5 * time.Minute
)

// DefaultScheduler default scheduler
var DefaultScheduler = singleton.NewSingleton[Scheduler](func() Scheduler {
	SchedulableShards = velaslices.Filter(SchedulableShards, func(s string) bool { return len(s) > 0 })
	if len(SchedulableShards) > 0 {
		klog.Infof("staticScheduler initialized")
		return NewStaticScheduler(SchedulableShards)
	}
	klog.Infof("dynamicDiscoveryScheduler initialized")
	return NewDynamicDiscoveryScheduler(meta.Name, DynamicDiscoverySchedulerResyncPeriod)
})
