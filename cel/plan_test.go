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

var planTree = map[string]interface{}{
	"url":   "$(alpha.host):$(alpha.port)",
	"peers": []interface{}{`$(peer.db.at("east").host)`},
	"plain": "text",
	"count": 3,
}

func TestPlanCollectsEveryReadWithItsProperty(t *testing.T) {
	p, err := testEngine(t).Plan(planTree)
	require.NoError(t, err)

	var alpha []string
	for _, r := range p.Reads("alpha") {
		alpha = append(alpha, r.Property+" <- "+r.String())
	}
	require.Equal(t, []string{"url <- alpha.host", "url <- alpha.port"}, alpha)
	require.Len(t, p.Reads("peer"), 1)
	require.Empty(t, p.Reads("beta"))

	var exprs []string
	for _, x := range p.Expressions() {
		exprs = append(exprs, fmt.Sprintf("%s %s whole=%v reads=%d", x.Property, x.Expr, x.Whole, len(x.Reads)))
	}
	require.Equal(t, []string{
		`peers[0] peer.db.at("east").host whole=true reads=1`,
		"url alpha.host whole=false reads=1",
		"url alpha.port whole=false reads=1",
	}, exprs)
}

// Admission wants every fault in one pass, each with the property it is in.
func TestPlanReportsEveryBadExpression(t *testing.T) {
	_, err := testEngine(t).Plan(map[string]interface{}{
		"a": "$(alpha.x",
		"b": "$(alpha.)",
		"c": "$(alpha.ok)",
	})
	var errs CheckErrors
	require.True(t, errors.As(err, &errs), "%v", err)
	require.Len(t, errs, 2)
	require.Equal(t, "a", errs[0].Property)
	require.Equal(t, "b", errs[1].Property)
}

func TestCheckRefusesRootsOutsideTheSurfaceAndRunsEachChecker(t *testing.T) {
	p, err := testEngine(t).Plan(planTree)
	require.NoError(t, err)

	calls := map[string]int{}
	checker := func(root string) Checker {
		return CheckerFunc(func(reads []Read) []CheckError {
			calls[root]++
			if root == "alpha" {
				return []CheckError{{Property: reads[1].Property, Read: reads[1], Err: errors.New("port is not offered")}}
			}
			return nil
		})
	}
	errs := p.Check([]string{"alpha"}, map[string]Checker{"alpha": checker("alpha"), "beta": checker("beta")})
	require.Len(t, errs, 2)
	require.ErrorContains(t, errs[0], `"peer" cannot be read here; this surface permits "alpha"`)
	require.Equal(t, "peers[0]", errs[0].Property)
	require.ErrorContains(t, errs[1], "url: alpha.port: port is not offered")
	require.Equal(t, map[string]int{"alpha": 1}, calls, "one call per read root, none for a root nothing reads")
}

// A plan is parsed once and can be evaluated more than once.
func TestPlanEvaluatesAndIsReusable(t *testing.T) {
	p, err := testEngine(t).Plan(planTree)
	require.NoError(t, err)
	resolvers := map[string]Resolver{
		"alpha": Static(map[string]interface{}{"host": "db", "port": 5432}),
		"peer": Static(map[string]interface{}{"db": map[string]interface{}{
			QualifiedKey: map[string]interface{}{"at:east": map[string]interface{}{"host": "db.east"}},
		}}),
	}
	want := map[string]interface{}{"url": "db:5432", "peers": []interface{}{"db.east"}, "plain": "text", "count": 3}
	for i := 0; i < 2; i++ {
		out, err := p.Eval(context.Background(), resolvers, TreeOptions{})
		require.NoError(t, err)
		require.Equal(t, want, out)
	}
	require.Equal(t, "$(alpha.host):$(alpha.port)", planTree["url"])
}

// A JSON document is planned once for its bytes, and the shared plan is only
// ever read: evaluating it leaves it as it was.
func TestPlanJSONSharesOnePlanPerDocument(t *testing.T) {
	e := testEngine(t)
	doc := []byte(`{"url":"$(alpha.host)","n":1}`)
	first, err := e.PlanJSON(doc)
	require.NoError(t, err)
	again, err := e.PlanJSON(append([]byte(nil), doc...))
	require.NoError(t, err)
	require.Same(t, first, again)

	other, err := e.PlanJSON([]byte(`{"url":"$(alpha.port)"}`))
	require.NoError(t, err)
	require.NotSame(t, first, other)

	for i := 0; i < 2; i++ {
		out, err := first.Eval(context.Background(), map[string]Resolver{"alpha": Static(map[string]interface{}{"host": "db"})}, TreeOptions{})
		require.NoError(t, err)
		require.Equal(t, map[string]interface{}{"url": "db", "n": float64(1)}, out)
	}
}

// A document that is not JSON, or holds an expression that does not compile,
// is reported every time rather than remembered.
func TestPlanJSONDoesNotCacheFailures(t *testing.T) {
	e := testEngine(t)
	_, err := e.PlanJSON([]byte(`{"url":`))
	require.Error(t, err)
	var faults CheckErrors
	require.False(t, errors.As(err, &faults), "malformed JSON is not an expression fault")

	bad := []byte(`{"url":"$(alpha."}`)
	for i := 0; i < 2; i++ {
		_, err = e.PlanJSON(bad)
		require.True(t, errors.As(err, &faults), "%v", err)
	}
}
