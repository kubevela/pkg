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
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TimeoutOptions timeout options for controller client
type TimeoutOptions struct {
	RequestTimeout            time.Duration
	LongRunningRequestTimeout time.Duration
	MutatingRequestTimeout    time.Duration
}

// WithTimeout return timeout context if RequestTimeout is greater than 0
func (in *TimeoutOptions) WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if in.RequestTimeout > 0 {
		return context.WithTimeout(ctx, in.RequestTimeout)
	}
	return ctx, func() {}
}

// WithLongRunningTimeout return timout context if LongRunningRequestTimeout is greater than 0
func (in *TimeoutOptions) WithLongRunningTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if in.LongRunningRequestTimeout > 0 {
		return context.WithTimeout(ctx, in.LongRunningRequestTimeout)
	}
	return in.WithTimeout(ctx)
}

// WithMutatingTimeout return timout context if WithMutatingTimeout is greater than 0
func (in *TimeoutOptions) WithMutatingTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if in.MutatingRequestTimeout > 0 {
		return context.WithTimeout(ctx, in.MutatingRequestTimeout)
	}
	return in.WithTimeout(ctx)
}

// TimeoutClient add timeout limit for requests
type TimeoutClient struct {
	client.Client
	TimeoutOptions
}

// Get .
func (in *TimeoutClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	ctx, cancel := in.WithTimeout(ctx)
	defer cancel()
	return in.Client.Get(ctx, key, obj)
}

// List .
func (in *TimeoutClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	ctx, cancel := in.WithLongRunningTimeout(ctx)
	defer cancel()
	return in.Client.List(ctx, list, opts...)
}

// Create .
func (in *TimeoutClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	ctx, cancel := in.WithMutatingTimeout(ctx)
	defer cancel()
	return in.Client.Create(ctx, obj, opts...)
}

// Delete .
func (in *TimeoutClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	ctx, cancel := in.WithMutatingTimeout(ctx)
	defer cancel()
	return in.Client.Delete(ctx, obj, opts...)
}

// Update .
func (in *TimeoutClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	ctx, cancel := in.WithMutatingTimeout(ctx)
	defer cancel()
	return in.Client.Update(ctx, obj, opts...)
}

// Patch .
func (in *TimeoutClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	ctx, cancel := in.WithMutatingTimeout(ctx)
	defer cancel()
	return in.Client.Patch(ctx, obj, patch, opts...)
}

// DeleteAllOf .
func (in *TimeoutClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	ctx, cancel := in.WithMutatingTimeout(ctx)
	defer cancel()
	return in.Client.DeleteAllOf(ctx, obj, opts...)
}

// Status .
func (in *TimeoutClient) Status() client.StatusWriter {
	return &TimeoutStatusWriter{
		StatusWriter:   in.Client.Status(),
		TimeoutOptions: in.TimeoutOptions,
	}
}

// TimeoutStatusWriter add timeout limit for requests
type TimeoutStatusWriter struct {
	client.StatusWriter
	TimeoutOptions
}

// Update .
func (in *TimeoutStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	ctx, cancel := in.WithMutatingTimeout(ctx)
	defer cancel()
	return in.StatusWriter.Update(ctx, obj, opts...)
}

// Patch .
func (in *TimeoutStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	ctx, cancel := in.WithMutatingTimeout(ctx)
	defer cancel()
	return in.StatusWriter.Patch(ctx, obj, patch, opts...)
}

// DefaultTimeoutClientOptions options for default timeout
var DefaultTimeoutClientOptions = &TimeoutOptions{
	RequestTimeout:            10 * time.Second,
	LongRunningRequestTimeout: 30 * time.Second,
	MutatingRequestTimeout:    15 * time.Second,
}

// WrapDefaultTimeoutClient wrap client with DefaultTimeoutClientOptions
func WrapDefaultTimeoutClient(c client.Client) client.Client {
	return &TimeoutClient{
		Client:         c,
		TimeoutOptions: *DefaultTimeoutClientOptions,
	}
}
