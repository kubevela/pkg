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

package cuex_test

import (
	"context"
	"os"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// Working out what a call reads must not change the value the calls run in.
// Following a call's references evaluates its inputs, and in the value the
// providers are given that left kubevela/workflow's share-cloud-resource step
// failing the second call's result: it came back bottom, and the calls after
// it were never found. The fixture is that step's package, reduced.
func TestWorkingOutReadsLeavesTheValueWhole(t *testing.T) {
	src, err := os.ReadFile("testdata/analysis/op.cue")
	require.NoError(t, err)
	var ran []string
	answer := func(do string, fill func(cue.Value) cue.Value) cuexruntime.ProviderFn {
		return legacyFn(func(value cue.Value) (cue.Value, error) {
			ran = append(ran, do)
			return fill(value), nil
		})
	}
	same := func(v cue.Value) cue.Value { return v }
	pkg, err := cuexruntime.NewInternalPackage("op", string(src), map[string]cuexruntime.ProviderFn{
		"load-policies": answer("load-policies", func(v cue.Value) cue.Value {
			return v.FillPath(cue.ParsePath("value"), map[string]any{"bindings": map[string]any{"type": "env-binding"}})
		}),
		"make-placement-decisions": answer("make-placement-decisions", func(v cue.Value) cue.Value {
			return v.FillPath(cue.ParsePath("outputs.decisions"), []any{map[string]any{"cluster": "local", "namespace": ""}})
		}),
		"load-terraform-components": answer("load-terraform-components", same),
	})
	require.NoError(t, err)

	// The reduced package does not render a whole value, which is not what is
	// under test: the calls it makes are.
	_, err = cuex.NewCompilerWithInternalPackages(pkg).CompileString(context.Background(), `
import "vela/op"
app: op.#ShareCloudResource & {}
`)
	require.NoError(t, err)
	require.Equal(t, []string{"load-policies", "make-placement-decisions", "load-terraform-components"}, ran,
		"every call runs, each once")
}
