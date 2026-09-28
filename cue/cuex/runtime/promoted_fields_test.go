/*
Copyright 2026 The KubeVela Authors.

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

package runtime_test

import (
	"context"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex/runtime"
)

type embedded struct {
	Extra string `json:"extra"`
}

// promotedInput reaches a field through an embedded type that is itself
// unexported. json still fills Extra from the node, so the shortcut that
// marshals $params alone has to see it.
type promotedInput struct {
	embedded
	Params string `json:"$params"`
}

type promotedOutput struct {
	Returns string `json:"$returns"`
}

// TestPromotedFieldIsNotDropped: only $params is marshalled where the input
// type asks for nothing else, so anything else it can be given has to count.
// An embedded type promotes its exported fields whether or not the type it
// came from is exported, and skipping it as unexported loses them in silence.
func TestPromotedFieldIsNotDropped(t *testing.T) {
	var saw promotedInput
	fn := runtime.GenericProviderFn[promotedInput, promotedOutput](
		func(_ context.Context, in *promotedInput) (*promotedOutput, error) {
			saw = *in
			return &promotedOutput{Returns: in.Params}, nil
		})

	v := cuecontext.New().CompileString(`{$params: "p", extra: "e"}`)
	_, err := fn.Call(context.Background(), v)
	require.NoError(t, err)
	require.Equal(t, "p", saw.Params)
	require.Equal(t, "e", saw.Extra, "a promoted field must reach the function")
}
