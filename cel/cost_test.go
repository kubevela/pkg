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
	"testing"

	"github.com/stretchr/testify/require"
)

// list is source.items with n entries, for expressions whose cost grows with it.
func list(n int) map[string]interface{} {
	items := make([]interface{}, n)
	for i := range items {
		items[i] = i
	}
	return map[string]interface{}{"source": map[string]interface{}{"items": items}}
}

func TestAnExpressionOverTheCostLimitFails(t *testing.T) {
	// 200^3 iterations: millions of operations, well past the limit.
	_, err := fixture.Eval(fixture.DynEnv(), "source.items.all(i, source.items.all(j, source.items.all(k, true)))", list(200))
	require.ErrorContains(t, err, "exceeds the evaluation cost limit")
}

func TestAnOrdinaryExpressionIsWellWithinTheCostLimit(t *testing.T) {
	out, err := fixture.Eval(fixture.DynEnv(), "source.items.filter(i, i > 10).map(i, i * 2).size()", list(1000))
	require.NoError(t, err)
	require.EqualValues(t, 989, out)
}

func TestACancelledContextStopsAPlansEvaluation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fixture.EvalTree(ctx, map[string]interface{}{"v": "$(source.items.all(i, source.items.all(j, true)))"},
		map[string]Resolver{"source": Static(list(300)["source"])}, TreeOptions{})
	require.ErrorIs(t, err, context.Canceled)
}
