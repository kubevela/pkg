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
