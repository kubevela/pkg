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
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/kubevela/pkg/monitor/metrics"
	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/k8s"
	velaruntime "github.com/kubevela/pkg/util/runtime"
)

const (
	// ControllerClientRequestLatencyKey metrics key for recording time cost
	// of controller client requests
	ControllerClientRequestLatencyKey = "controller_client_request_time_seconds"
)

var (
	// controllerClientRequestLatency the client request latency metrics
	// It records the latency for calling monitorClient functions and
	// monitorCache functions
	controllerClientRequestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: metrics.KubeVelaSubsystem,
			Name:      ControllerClientRequestLatencyKey,
			Help:      "client request duration for kubevela controllers",
			Buckets:   metrics.FineGrainedBuckets,
		}, []string{"controller", "cluster", "verb", "kind", "apiVersion", "unstructured"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(controllerClientRequestLatency)
}

// monitor creates a callback to call when function ends
// It reports the execution duration for the function call
func monitor(ctx context.Context, verb string, obj runtime.Object) func() {
	begin := time.Now()
	cluster, _ := multicluster.ClusterFrom(ctx)
	return func() {
		v := time.Since(begin).Seconds()
		controllerClientRequestLatency.WithLabelValues(
			velaruntime.GetController(ctx),
			cluster,
			verb,
			k8s.GetKindForObject(obj, true),
			obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			fmt.Sprintf("%t", k8s.IsUnstructuredObject(obj)),
		).Observe(v)
	}
}

// monitorCache records time costs in metrics when execute function calls
type monitorCache struct {
	cache.Cache
}

func (c *monitorCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	cb := monitor(ctx, "GetCache", obj)
	defer cb()
	return c.Cache.Get(ctx, key, obj)
}

func (c *monitorCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	cb := monitor(ctx, "ListCache", list)
	defer cb()
	return c.Cache.List(ctx, list, opts...)
}

// monitorClient records time costs in metrics when execute function calls
type monitorClient struct {
	client.Client
}

func (c *monitorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	cb := monitor(ctx, "Get", obj)
	defer cb()
	return c.Client.Get(ctx, key, obj)
}

func (c *monitorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	cb := monitor(ctx, "List", list)
	defer cb()
	return c.Client.List(ctx, list, opts...)
}

func (c *monitorClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	cb := monitor(ctx, "Create", obj)
	defer cb()
	return c.Client.Create(ctx, obj, opts...)
}

func (c *monitorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	cb := monitor(ctx, "Delete", obj)
	defer cb()
	return c.Client.Delete(ctx, obj, opts...)
}

func (c *monitorClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	cb := monitor(ctx, "Update", obj)
	defer cb()
	return c.Client.Update(ctx, obj, opts...)
}

func (c *monitorClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	cb := monitor(ctx, "Patch", obj)
	defer cb()
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *monitorClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	cb := monitor(ctx, "DeleteAllOf", obj)
	defer cb()
	return c.Client.DeleteAllOf(ctx, obj, opts...)
}

func (c *monitorClient) Status() client.StatusWriter {
	return &monitorStatusWriter{c.Client.Status()}
}

// monitorStatusWriter records time costs in metrics when execute function calls
type monitorStatusWriter struct {
	client.StatusWriter
}

func (w *monitorStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	cb := monitor(ctx, "StatusUpdate", obj)
	defer cb()
	return w.StatusWriter.Update(ctx, obj, opts...)
}

func (w *monitorStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	cb := monitor(ctx, "StatusPatch", obj)
	defer cb()
	return w.StatusWriter.Patch(ctx, obj, patch, opts...)
}
