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

package base64_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex/providers/base64"
)

func TestBase64(t *testing.T) {
	ctx := context.Background()
	v := &base64.Var{Input: "example"}
	_v, err := base64.Encode(ctx, v)
	require.NoError(t, err)
	require.Equal(t, "ZXhhbXBsZQ==", _v.Output)

	v = &base64.Var{Input: "ZXhhbXBsZQ=="}
	_v, err = base64.Decode(ctx, v)
	require.NoError(t, err)
	require.Equal(t, "example", _v.Output)
}
