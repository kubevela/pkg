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
	"sync/atomic"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

var probeRuns atomic.Int64

type probeIn struct {
	Params string `json:"$params"`
}
type probeOut struct {
	Returns string `json:"$returns"`
}
type richIn struct {
	Params string `json:"$params"`
}
type richOut struct {
	Returns struct {
		Num float64 `json:"num"`
	} `json:"$returns"`
}

func probePackage() cuexruntime.Package {
	pkg, err := cuexruntime.NewInternalPackage("probe", `
package probe

#Do: {
	#do:       "do"
	#provider: "probe"
	$params:   string
	$returns?: string
}

#Rich: {
	#do:       "rich"
	#provider: "probe"
	$params:   string
	$returns?: {num: float}
}
`, map[string]cuexruntime.ProviderFn{
		"do": cuexruntime.GenericProviderFn[probeIn, probeOut](
			func(_ context.Context, in *probeIn) (*probeOut, error) {
				probeRuns.Add(1)
				return &probeOut{Returns: in.Params}, nil
			}),
		"rich": cuexruntime.GenericProviderFn[richIn, richOut](
			func(_ context.Context, in *richIn) (*richOut, error) {
				out := &richOut{}
				out.Returns.Num = 2
				return out, nil
			}),
	})
	if err != nil {
		panic(err)
	}
	return pkg
}

func TestADuplicateDeclarationStillCallsOnce(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(probePackage())
	probeRuns.Store(0)
	v, err := c.CompileString(context.Background(), `
import "vela/probe"
a: probe.#Do
a: $params: "once"
out: a.$returns
`)
	require.NoError(t, err)
	_, err = v.LookupPath(cue.ParsePath("out")).String()
	require.NoError(t, err)
	t.Logf("PROBE1 duplicate-declaration: provider ran %d time(s)", probeRuns.Load())
	require.EqualValues(t, 1, probeRuns.Load())
}

// The route the duplicate declaration above actually arrives by.
//
// WithExtraData reaches a template through its source: it appends
// "key: <data>" for the parameter to be read back. For a key under a field
// the template already declares, that is a second declaration of that
// field, so a template declaring template: with a call under it had every
// such call made twice. No template author has to do anything unusual for
// this: kubevela renders every Config through
// cuex.WithExtraData("template.parameter", ...) in
// pkg/cue/script/template.go, from the config controller, the config
// factory and the validating webhook, so the webhook made the calls a
// second time on top.
//
// Measured on 31c8736, which is #144 as merged: two runs with the extra
// data and one without.
func TestExtraDataForADeclaredFieldStillCallsOnce(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(probePackage())
	// the shape a config template has: the calls live under template, and
	// the parameters are supplied into template.parameter
	src := `
import "vela/probe"
template: {
	parameter: {word: *"default" | string}
	out: probe.#Do & {$params: parameter.word}
}
`
	for _, tc := range []struct {
		name string
		opts []cuex.CompileOption
		want string
	}{
		{"on its own", nil, "default"},
		{
			"with the parameters supplied the way a Config render supplies them",
			[]cuex.CompileOption{
				cuex.WithExtraData("template.parameter", map[string]any{"word": "given"}),
			},
			"given",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probeRuns.Store(0)
			v, err := c.CompileStringWithOptions(context.Background(), src, tc.opts...)
			require.NoError(t, err)
			got, err := v.LookupPath(cue.ParsePath("template.out.$returns")).String()
			require.NoError(t, err)
			require.Equal(t, tc.want, got,
				"the parameters have to reach the call")
			require.EqualValues(t, 1, probeRuns.Load(),
				"the call has to be made once, however the field came to be declared twice")
		})
	}
}

func TestProbeResultTypeDoesNotDependOnCompany(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(probePackage())
	ctx := context.Background()
	alone, err := c.CompileString(ctx, `
import "vela/probe"
a: probe.#Rich & {$params: "x"}
out: a.$returns.num
`)
	require.NoError(t, err)
	aloneJSON, aErr := alone.LookupPath(cue.ParsePath("out")).MarshalJSON()

	together, err := c.CompileString(ctx, `
import "vela/probe"
a: probe.#Rich & {$params: "x"}
b: probe.#Rich & {$params: "y"}
out: a.$returns.num
`)
	require.NoError(t, err)
	togetherJSON, tErr := together.LookupPath(cue.ParsePath("out")).MarshalJSON()

	t.Logf("PROBE2 result-type: alone=%s/%v together=%s/%v", aloneJSON, aErr, togetherJSON, tErr)
	require.NoError(t, aErr)
	require.NoError(t, tErr)
	require.Equal(t, string(aloneJSON), string(togetherJSON))
}

func TestProbeCallCopiedThroughASelector(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(probePackage())
	v, err := c.CompileString(context.Background(), `
import "vela/probe"
#tpl: inner: probe.#Do & {$params: "a"}
second: #tpl.inner
out: second.$returns
`)
	require.NoError(t, err)
	got, gErr := v.LookupPath(cue.ParsePath("out")).String()
	t.Logf("PROBE3 copied-through-selector: out=%q err=%v", got, gErr)
	require.NoError(t, gErr)
	require.Equal(t, "a", got)
}

func TestProbeOptionalHiddenCall(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(probePackage())
	probeRuns.Store(0)
	_, err := c.CompileString(context.Background(), `
import "vela/probe"
_enc?: probe.#Do & {$params: "h"}
out: "x"
`)
	require.NoError(t, err)
	t.Logf("PROBE4 optional-hidden: provider ran %d time(s)", probeRuns.Load())
	require.EqualValues(t, 1, probeRuns.Load())
}

func TestProbeStepOrderAtTopLevel(t *testing.T) {
	var order []string
	pkg, err := cuexruntime.NewInternalPackage("order", `
package order

#Do: {
	#do:       "do"
	#provider: "order"
	$params:   string
	$returns?: string
}
`, map[string]cuexruntime.ProviderFn{
		"do": cuexruntime.GenericProviderFn[probeIn, probeOut](
			func(_ context.Context, in *probeIn) (*probeOut, error) {
				order = append(order, in.Params)
				return &probeOut{Returns: in.Params}, nil
			}),
	})
	require.NoError(t, err)

	c := cuex.NewCompilerWithInternalPackages(pkg)
	_, err = c.CompileString(context.Background(), `
import "vela/order"
b: order.#Do & {$params: "b"} @step(2)
a: order.#Do & {$params: "a"} @step(1)
out: "\(a.$returns)\(b.$returns)"
`)
	require.NoError(t, err)
	t.Logf("PROBE5 step-order: ran %v", order)
	require.Equal(t, []string{"a", "b"}, order)
}
