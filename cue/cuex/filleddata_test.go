package cuex_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

var r2Runs atomic.Int64

type r2In struct {
	Params string `json:"$params"`
}
type r2Out struct {
	Returns string `json:"$returns"`
}

func r2Package() cuexruntime.Package {
	pkg, err := cuexruntime.NewInternalPackage("r2", `
package r2

#Do: {
	#do:       "do"
	#provider: "r2"
	$params:   string
	$returns?: string
}
`, map[string]cuexruntime.ProviderFn{
		"do": cuexruntime.GenericProviderFn[r2In, r2Out](
			func(_ context.Context, in *r2In) (*r2Out, error) {
				r2Runs.Add(1)
				return &r2Out{Returns: in.Params}, nil
			}),
	})
	if err != nil {
		panic(err)
	}
	return pkg
}

// Data filled in as syntax is compiled as CUE, so a "#do" in it is a
// definition and the field it lands in is a call. The file's own syntax
// never named that field as one, so the narrowed walk has to stand
// down, the same as it does for a cue.Value.
func TestR2SyntaxDataCarryingACall(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(r2Package())
	expr, err := parser.ParseExpr("-", `{#do: "do", #provider: "r2", $params: "injected"}`)
	require.NoError(t, err)

	r2Runs.Store(0)
	v, err := c.CompileStringWithOptions(context.Background(),
		"import \"vela/r2\"\nslot: _\nkeep: r2.#Do & {$params: \"named\"}\nout: keep.$returns\n",
		cuex.WithData("slot", expr))
	require.NoError(t, err)

	got := v.LookupPath(cue.ParsePath("slot.$returns"))
	t.Logf("R2-4 filled-as-syntax: slot.$returns exists=%v runs=%d", got.Exists(), r2Runs.Load())
	require.True(t, got.Exists(),
		"a call filled in as syntax should run like one filled in as a value")
}

// A hidden field holding a call makes lookup decline and the walk falls
// back to reading the root's fields, which is the thing the narrowing
// exists to avoid, and hidden is how templates usually hold a call.
//
// It costs nothing measurable. The same calls over the same four
// hundred field manifest come out level whichever way they are named,
// so the fallback is not the expense it reads as. Kept because the
// reasoning says it should be and the measurement says it is not, and
// the next person to read that code will wonder the same thing.
func TestR2HiddenFieldsAreNotSlower(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(r2Package())
	ctx := context.Background()

	// the same calls and the same manifest, named two ways
	build := func(n, manifest int, hidden bool) string {
		var b strings.Builder
		b.WriteString("import \"vela/r2\"\n")
		prefix := "x"
		if hidden {
			prefix = "_x"
		}
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "%s%d: r2.#Do & {$params: \"p%d\"}\n", prefix, i, i)
		}
		// at the top level, which is what enumerate reads
		for i := 0; i < manifest; i++ {
			fmt.Fprintf(&b, "m%d: {a: \"%d\", b: {c: \"%d\"}, d: [1, 2, 3]}\n", i, i, i)
		}
		fmt.Fprintf(&b, "output: first: %s0.$returns\n", prefix)
		return b.String()
	}

	for _, n := range []int{10, 50} {
		const manifest = 400
		timeIt := func(hidden bool) time.Duration {
			best := time.Duration(1<<62 - 1)
			for i := 0; i < 5; i++ {
				src := build(n, manifest, hidden)
				start := time.Now()
				_, err := c.CompileString(ctx, src)
				require.NoError(t, err)
				if d := time.Since(start); d < best {
					best = d
				}
			}
			return best
		}
		visible, hidden := timeIt(false), timeIt(true)
		ratio := float64(hidden) / float64(visible)
		t.Logf("R2-5 %2d calls over a %d field manifest: visible %8s  hidden %8s  %.2fx",
			n, manifest, visible.Round(time.Microsecond), hidden.Round(time.Microsecond), ratio)
		// A loose bound on purpose. This is two wall clock readings on
		// whatever machine happens to run it, so it is here to catch an
		// order of magnitude and nothing finer; the ratio in the log is
		// the thing worth reading.
		require.Less(t, ratio, 4.0,
			"naming a call's field hidden should not cost several times as much")
	}
}
