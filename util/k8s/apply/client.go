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

package apply

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/jsonutil"
)

// Client override the create/update/patch interface and handle status update simultaneously
type Client struct {
	client.Client
}

func (in *Client) hasUnstructuredStatus(obj client.Object) (any, bool) {
	if o, isUnstructured := obj.(*unstructured.Unstructured); isUnstructured && o.Object != nil {
		status, hasStatus := o.Object["status"]
		return status, hasStatus
	}
	if m, _ := jsonutil.AsType[map[string]interface{}](obj); m != nil {
		status, hasStatus := (*m)["status"]
		return status, hasStatus
	}
	return nil, false
}

func (in *Client) setUnstructuredStatus(obj client.Object, status any) {
	if o, isUnstructured := obj.(*unstructured.Unstructured); isUnstructured && o.Object != nil {
		o.Object["status"] = status
	} else if m, _ := jsonutil.AsType[map[string]interface{}](obj); m != nil {
		(*m)["status"] = status
		_ = jsonutil.CopyInto(*m, obj)
	}
}

// Create .
func (in *Client) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	status, hasStatus := in.hasUnstructuredStatus(obj)
	if err := in.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}
	_opts := &client.CreateOptions{}
	for _, opt := range opts {
		opt.ApplyToCreate(_opts)
	}
	if hasStatus && len(_opts.DryRun) == 0 {
		in.setUnstructuredStatus(obj, status)
		return in.Client.Status().Update(ctx, obj)
	}
	return nil
}

// Update .
func (in *Client) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	status, hasStatus := in.hasUnstructuredStatus(obj)
	if err := in.Client.Update(ctx, obj, opts...); err != nil {
		return err
	}
	if hasStatus {
		in.setUnstructuredStatus(obj, status)
		return in.Client.Status().Update(ctx, obj)
	}
	return nil
}

// Patch .
func (in *Client) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	status, hasStatus := in.hasUnstructuredStatus(obj)
	if err := in.Client.Patch(ctx, obj, patch, opts...); err != nil {
		return err
	}
	if hasStatus {
		in.setUnstructuredStatus(obj, status)
		return in.Client.Status().Patch(ctx, obj, patch)
	}
	return nil
}
