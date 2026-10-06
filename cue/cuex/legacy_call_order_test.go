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
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// deepNesting is past cuex's maxReferenceDepth (100), the depth it looks for
// references to; a test reaching it must nest deeper than that bound.
const deepNesting = 110

// legacyFn is a provider in the shape kubevela/workflow's legacy providers take: it
// is handed the whole call, with its inputs at the top level and no $params.
type legacyFn func(value cue.Value) (cue.Value, error)

func (fn legacyFn) Call(_ context.Context, value cue.Value) (cue.Value, error) {
	return fn(value)
}

type seqParams struct {
	Params struct {
		V int `json:"v"`
	} `json:"$params"`
}

type seqReturns struct {
	Returns struct {
		V int `json:"v"`
	} `json:"$returns"`
}

// seqCompiler offers a new-style call (#New, $params to $returns), a legacy call that
// produces output on its own fields (#Make), and legacy calls recording what their
// input held when they ran: `ready` (#Check), the hidden `_ready` (#CheckHidden), or
// a leaf nested deepNesting levels down (#CheckDeep). The package also declares a top-level
// bottom, as kubevela/workflow's legacy op package does (NoExist: _|_), which is
// what a call's inputs must be read without reaching into.
func seqCompiler(t *testing.T, seen *[]bool) *cuex.Compiler {
	var mu sync.Mutex
	pkg, err := cuexruntime.NewInternalPackage("seq", `
package seq

#New: {
	#do:       "new"
	#provider: "seq"
	$params: v: int
	$returns?: v: int
}

#Make: {
	#do:       "make"
	#provider: "seq"
	value?: {...}
}

#Check: {
	#do:       "check"
	#provider: "seq"
	ready: *false | bool
}

#CheckHidden: {
	#do:       "check-hidden"
	#provider: "seq"
	...
}

#CheckDeep: {
	#do:       "check-deep"
	#provider: "seq"
	...
}

NoExist: _|_`, map[string]cuexruntime.ProviderFn{
		"new": cuexruntime.GenericProviderFn[seqParams, seqReturns](func(_ context.Context, in *seqParams) (*seqReturns, error) {
			out := &seqReturns{}
			out.Returns.V = in.Params.V
			return out, nil
		}),
		// Reads its input from the hidden field _ready of the template's own package.
		"check-hidden": legacyFn(func(value cue.Value) (cue.Value, error) {
			ready, _ := value.LookupPath(cue.MakePath(cue.Hid("_ready", "_"))).Bool()
			mu.Lock()
			*seen = append(*seen, ready)
			mu.Unlock()
			return value, nil
		}),
		// Reports whether the leaf deepNesting levels down under deep is resolved.
		"check-deep": legacyFn(func(value cue.Value) (cue.Value, error) {
			leaf := value.LookupPath(cue.ParsePath("deep" + strings.Repeat(".a", deepNesting)))
			_, err := leaf.Int64()
			mu.Lock()
			*seen = append(*seen, err == nil)
			mu.Unlock()
			return value, nil
		}),
		"make": legacyFn(func(value cue.Value) (cue.Value, error) {
			return value.FillPath(cue.ParsePath("value"), map[string]any{"v": 1}), nil
		}),
		// Decoded as kubevela/workflow's legacy providers decode their call: the
		// whole value, marshalled to JSON.
		"check": legacyFn(func(value cue.Value) (cue.Value, error) {
			bs, err := value.MarshalJSON()
			if err != nil {
				return value, err
			}
			var in struct {
				Ready bool `json:"ready"`
			}
			if err := json.Unmarshal(bs, &in); err != nil {
				return value, err
			}
			mu.Lock()
			*seen = append(*seen, in.Ready)
			mu.Unlock()
			return value, nil
		}),
	})
	require.NoError(t, err)
	return cuex.NewCompilerWithInternalPackages(pkg)
}

// A call with no $params reads its inputs from its top-level fields, so those are what
// it depends on: it must run after the calls they read, not alongside them.
func TestLegacyCallWaitsForTheCallItReads(t *testing.T) {
	// Each reader comes before what it reads, so traversal order alone cannot pass.
	cases := map[string]string{
		// workflow's request: op.#ConditionalWait & {continue: req.$returns != _|_}
		"a legacy call reading a new-style call's $returns": `
import "vela/seq"
wait: seq.#Check & {ready: req.$returns != _|_}
req:  seq.#New & {$params: v: 1}
`,
		// workflow's apply-job: a legacy wait reading what a legacy apply wrote
		"a legacy call reading another legacy call's output": `
import "vela/seq"
wait:  seq.#Check & {ready: apply.value != _|_}
apply: seq.#Make & {}
`,
		"a legacy call reading through a hidden field": `
import "vela/seq"
wait: seq.#CheckHidden & {_ready: req.$returns != _|_}
req:  seq.#New & {$params: v: 1}
`,
		// Its only reference sits past the depth references are looked for, so it is
		// never found: the call must wait on every peer to see it resolved.
		"a legacy call nested too deep to read": `
import "vela/seq"
wait: seq.#CheckDeep & {deep: ` + strings.Repeat("{a: ", deepNesting) + "req.$returns.v" + strings.Repeat("}", deepNesting) + `}
req:  seq.#New & {$params: v: 1}
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			var seen []bool
			_, err := seqCompiler(t, &seen).CompileString(context.Background(), src)
			require.NoError(t, err)
			require.Equal(t, []bool{true}, seen, "the check must run once, after what it reads")
		})
	}
}
