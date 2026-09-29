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

package cuex

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"
)

// The put-back race was run against a root built once and reused, which is
// not the root the resolver has: that one is freshly built and has only been
// walked, so whichever way the answers go in pays for the evaluation the walk
// did not force. This is the same four ways from the state the resolver is
// actually in.
func TestPutBackFromWhereTheResolverStands(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()
	providers := c.PackageManager.GetProviders()
	ctx := context.Background()
	const runs = 800
	const n = 8
	src := apportionSrc(n)

	// each way, timed from a root built and walked afresh, marshal included
	ways := []struct {
		name string
		put  func(value cue.Value, results []callResult, f *ast.File) cue.Value
	}{
		{"overlay and one unify, as today", func(value cue.Value, results []callResult, _ *ast.File) cue.Value {
			return applyResults(value, results)
		}},
		{"one FillPath of the lot", func(value cue.Value, results []callResult, _ *ast.File) cue.Value {
			whole := map[string]any{}
			for _, r := range results {
				whole[r.call.fill.Selectors()[0].String()] = map[string]any{"$returns": returnsOf(r)}
			}
			return value.FillPath(cue.Path{}, whole)
		}},
		{"one FillPath each", func(value cue.Value, results []callResult, _ *ast.File) cue.Value {
			out := value
			for _, r := range results {
				out = out.FillPath(r.call.fill, r.ret)
			}
			return out
		}},
		// the overlay exactly as today, but filled in rather than unified in:
		// same syntax, so same field order, and the only thing that changes is
		// which of the two CUE calls writes it
		{"the same overlay, filled instead of unified", func(value cue.Value, results []callResult, _ *ast.File) cue.Value {
			overlay := &overlayNode{}
			for _, r := range results {
				expr, ok := resultSyntax(value.Context(), r.ret, r.opaque)
				require.True(t, ok)
				require.True(t, overlay.set(r.call.fill, expr))
			}
			return value.FillPath(cue.Path{}, value.Context().BuildExpr(overlay.expr()))
		}},
	}

	t.Logf("%-34s %s", "way", "time")
	for _, way := range ways {
		var took time.Duration
		var check string
		for i := 0; i < runs; i++ {
			f, err := parser.ParseFile("-", src, parser.ParseComments)
			require.NoError(t, err)
			bi := build.NewContext().NewInstance("", nil)
			bi.Imports = imports
			require.NoError(t, bi.AddSyntax(f))
			value := cuecontext.New().BuildInstance(bi)
			pending := pendingCalls(value, map[string]bool{}, scopeOf(f, imports))
			require.Len(t, pending, n)
			var results []callResult
			for _, call := range pending {
				fn, err := providerFn(providers, call)
				require.NoError(t, err)
				ret, err := callProvider(ctx, fn, call)
				require.NoError(t, err)
				results = append(results, callResult{call: call, ret: ret})
			}

			s := time.Now()
			out := way.put(value, results, f)
			bs, err := out.MarshalJSON()
			took += time.Since(s)
			require.NoError(t, err)
			check = string(bs)
		}
		t.Logf("%-34s %6.0fus", way.name, float64(took.Nanoseconds())/float64(runs)/1000)
		require.Contains(t, check, "$returns", "every way has to actually answer")
	}
}

func returnsOf(r callResult) any {
	if v, ok := r.ret.(cue.Value); ok {
		var out any
		_ = v.LookupPath(cue.ParsePath("$returns")).Decode(&out)
		return out
	}
	bs, err := json.Marshal(r.ret)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(bs, &out); err != nil {
		return nil
	}
	return out["$returns"]
}
