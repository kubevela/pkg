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
package cel

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// recorder answers every read of its root from values, and remembers what it
// was asked.
type recorder struct {
	values interface{}
	calls  int
	reads  []string
}

func (r *recorder) Resolve(_ context.Context, reads []Read) (interface{}, error) {
	r.calls++
	for _, read := range reads {
		r.reads = append(r.reads, read.Property+" <- "+read.String())
	}
	return r.values, nil
}

func TestEvalTreeResolvesEachRootOnceWithEveryRead(t *testing.T) {
	e := testEngine(t)
	alpha := &recorder{values: map[string]interface{}{"host": "db", "port": 5432}}
	beta := &recorder{values: map[string]interface{}{}}
	tree := map[string]interface{}{
		"url":   "$(alpha.host):$(alpha.port)",
		"list":  []interface{}{"$(alpha.host)", "plain"},
		"count": 3,
	}

	out, err := e.EvalTree(context.Background(), tree, map[string]Resolver{"alpha": alpha, "beta": beta}, TreeOptions{})
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{
		"url":   "db:5432",
		"list":  []interface{}{"db", "plain"},
		"count": 3,
	}, out)
	require.Equal(t, 1, alpha.calls)
	require.Equal(t, []string{
		"list[0] <- alpha.host",
		"url <- alpha.host",
		"url <- alpha.port",
	}, alpha.reads, "every read, with the property it feeds, in a stable order")
	require.Zero(t, beta.calls, "a root nothing reads is not resolved")
	require.Equal(t, "$(alpha.host):$(alpha.port)", tree["url"], "the input is left alone")
}

func TestEvalTreeRefusesARootWithNoResolver(t *testing.T) {
	e := testEngine(t)
	_, err := e.EvalTree(context.Background(), map[string]interface{}{"a": "$(beta.x)"},
		map[string]Resolver{"alpha": Static(map[string]interface{}{})}, TreeOptions{})
	require.ErrorContains(t, err, `nothing resolves "beta" here`)
}

func TestEvalTreeKeepsTheResolversError(t *testing.T) {
	e := testEngine(t)
	waiting := ResolverFunc(func(context.Context, []Read) (interface{}, error) {
		return nil, fmt.Errorf("waiting for peer.db: %w", ErrNotReady)
	})
	_, err := e.EvalTree(context.Background(), map[string]interface{}{"a": "$(peer.db.x)"},
		map[string]Resolver{"peer": waiting}, TreeOptions{})
	require.True(t, errors.Is(err, ErrNotReady), "a host has to be able to tell waiting from failing: %v", err)
}

func TestEvalTreeRendersUnknownReadsWithTheHostsHook(t *testing.T) {
	e := testEngine(t)
	resolvers := map[string]Resolver{
		"alpha": Static(map[string]interface{}{"region": "eu"}),
		"peer":  Static(Unknown),
	}
	tree := map[string]interface{}{
		"whole": "$(peer.db.host)",
		"mixed": "$(alpha.region)-$(peer.db.host)",
		"plain": "$(alpha.region)",
	}

	_, err := e.EvalTree(context.Background(), tree, resolvers, TreeOptions{})
	require.ErrorContains(t, err, "has no value in this render")

	out, err := e.EvalTree(context.Background(), tree, resolvers, TreeOptions{
		Unknown: func(expr string, whole bool) (interface{}, error) {
			if whole {
				return "WHOLE<" + expr + ">", nil
			}
			return "<" + expr + ">", nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{
		"whole": "WHOLE<peer.db.host>",
		"mixed": "eu-<peer.db.host>",
		"plain": "eu",
	}, out)
}

func TestEvalTreeHonoursTypedValues(t *testing.T) {
	e := testEngine(t)
	out, err := e.EvalTree(context.Background(), map[string]interface{}{"r": "$(alpha.ratio * 2.0)"},
		map[string]Resolver{"alpha": Static(Typed(map[string]interface{}{"ratio": float64(2)}))}, TreeOptions{})
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"r": 4.0}, out)
}

func TestEvalTreeReportsAParseErrorBeforeResolving(t *testing.T) {
	e := testEngine(t)
	alpha := &recorder{values: map[string]interface{}{}}
	_, err := e.EvalTree(context.Background(), map[string]interface{}{"a": "$(alpha.x", "b": "$(alpha.y)"},
		map[string]Resolver{"alpha": alpha}, TreeOptions{})
	require.ErrorContains(t, err, "unterminated")
	require.Zero(t, alpha.calls)
}

// A host may reclassify an evaluation error, knowing which roots the
// expression reads.
func TestEvalTreeLetsTheHostReclassifyAnEvaluationError(t *testing.T) {
	e := testEngine(t)
	var gotRoots []string
	_, err := e.EvalTree(context.Background(), map[string]interface{}{"v": "x-$(peer.db.list[0])"},
		map[string]Resolver{"peer": Static(map[string]interface{}{"db": map[string]interface{}{"list": []interface{}{}}})},
		TreeOptions{OnEvalError: func(_ string, roots []string, err error) error {
			gotRoots = roots
			return fmt.Errorf("waiting: %w", ErrNotReady)
		}})
	require.True(t, errors.Is(err, ErrNotReady), "%v", err)
	require.Equal(t, []string{"peer"}, gotRoots)
}

// A dry run and a real render interpolate the same way: escapes collapse and
// other fragments evaluate, only the unknown read differs.
func TestUnknownReadsInterpolateLikeARealRender(t *testing.T) {
	e := testEngine(t)
	out, err := e.EvalTree(context.Background(), map[string]interface{}{"v": "$$(keep) $(alpha.n + 1)/$(peer.db.host)"},
		map[string]Resolver{"alpha": Static(map[string]interface{}{"n": 1}), "peer": Static(Unknown)},
		TreeOptions{Unknown: func(expr string, _ bool) (interface{}, error) { return "<" + expr + ">", nil }})
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"v": "$(keep) 2/<peer.db.host>"}, out)
}

// The hook sees each root once, and returning nil does not hide the failure.
func TestOnEvalErrorSeesEachRootOnceAndCannotHideAFailure(t *testing.T) {
	e := testEngine(t)
	var gotRoots []string
	_, err := e.EvalTree(context.Background(), map[string]interface{}{"v": "$(peer.a[0] + peer.b[0])"},
		map[string]Resolver{"peer": Static(map[string]interface{}{"a": []interface{}{}, "b": []interface{}{}})},
		TreeOptions{OnEvalError: func(_ string, roots []string, _ error) error {
			gotRoots = roots
			return nil
		}})
	require.Equal(t, []string{"peer"}, gotRoots)
	require.Error(t, err, "a nil from the hook keeps the evaluation's own error")
}
