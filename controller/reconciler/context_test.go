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

package reconciler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReconcileContext(t *testing.T) {
	baseCtx := context.Background()
	t0 := time.Now().Add(ReconcileTimeout)
	ctx, _ := NewReconcileContext(baseCtx)
	_ctx, ok := BaseContextFrom(ctx)
	require.True(t, ok)
	require.Equal(t, baseCtx, _ctx)
	t1 := time.Now().Add(ReconcileTimeout)
	ddl, ok := ctx.Deadline()
	require.True(t, ok)
	require.True(t, ddl.After(t0))
	require.True(t, ddl.Before(t1))
}

func TestReconcileTerminationContext(t *testing.T) {
	t0 := time.Now()
	baseCtx := context.Background()

	ctx, cancel := context.WithDeadline(WithBaseContext(baseCtx, baseCtx), t0)
	defer cancel()
	_ctx, ok := BaseContextFrom(ctx)
	require.True(t, ok)
	require.Equal(t, baseCtx, _ctx)
	_ctx, _ = NewReconcileTerminationContext(ctx)
	t1, ok := _ctx.Deadline()
	require.True(t, ok)
	require.True(t, t1.After(t0))

	ctx, cancel = context.WithDeadline(baseCtx, t0)
	defer cancel()
	_ctx, _ = NewReconcileTerminationContext(ctx)
	t1, ok = _ctx.Deadline()
	require.True(t, ok)
	require.True(t, t1.Equal(t0))
}
