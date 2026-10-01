package cuex_test

import (
	"context"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

type resultTypesIn struct {
	Params string `json:"$params"`
}
type resultTypesOut struct {
	Returns struct {
		Whole float64 `json:"whole"`
		Blob  []byte  `json:"blob"`
	} `json:"$returns"`
}

func resultTypesPackage() cuexruntime.Package {
	pkg, err := cuexruntime.NewInternalPackage("rtypes", `
package rtypes

#Do: {
	#do:       "do"
	#provider: "rtypes"
	$params:   string
	$returns?: {whole: float, blob: bytes}
}
`, map[string]cuexruntime.ProviderFn{
		"do": cuexruntime.GenericProviderFn[resultTypesIn, resultTypesOut](
			func(_ context.Context, in *resultTypesIn) (*resultTypesOut, error) {
				out := &resultTypesOut{}
				out.Returns.Whole = 2
				out.Returns.Blob = []byte("hi")
				return out, nil
			}),
	})
	if err != nil {
		panic(err)
	}
	return pkg
}

// A call reached by indexing into a definition, and a result whose type
// has to come out the same however many calls shared the pass.
//
// The index into a definition is the one worth having: elsewhere there is
// a selector onto a definition and an index into a list, and those are not
// the same path through the code.
func TestACallIndexedOutOfADefinition(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(resultTypesPackage())
	ctx := context.Background()

	for _, tc := range []struct{ name, src string }{
		{"selector onto a definition", `
import "vela/rtypes"
#outer: {t: rtypes.#Do & {$params: "a"}}
second: #outer.t
out: second.$returns.whole`},
		{"index into a definition", `
import "vela/rtypes"
#m: {"k": rtypes.#Do & {$params: "a"}}
second: #m["k"]
out: second.$returns.whole`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := c.CompileString(ctx, tc.src)
			require.NoError(t, err)
			got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
			require.NoError(t, err, "the call should have run")
			require.JSONEq(t, "2", string(got))
		})
	}
}

// A whole float and a []byte keep their kind whether one call ran or
// two, which is what the declared $returns above requires: a template
// saying float and bytes would be a silent bottom otherwise.
func TestAResultsTypeDoesNotDependOnPassWidth(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(resultTypesPackage())
	ctx := context.Background()

	read := func(src string) (cue.Kind, cue.Kind) {
		v, err := c.CompileString(ctx, src)
		require.NoError(t, err)
		whole := v.LookupPath(cue.ParsePath("a.$returns.whole"))
		blob := v.LookupPath(cue.ParsePath("a.$returns.blob"))
		require.NoError(t, whole.Err())
		require.NoError(t, blob.Err())
		return whole.Kind(), blob.Kind()
	}

	oneWhole, oneBlob := read(`
import "vela/rtypes"
a: rtypes.#Do & {$params: "x"}`)
	twoWhole, twoBlob := read(`
import "vela/rtypes"
a: rtypes.#Do & {$params: "x"}
b: rtypes.#Do & {$params: "y"}`)

	t.Logf("one call: whole=%v blob=%v | two calls: whole=%v blob=%v",
		oneWhole, oneBlob, twoWhole, twoBlob)
	require.Equal(t, oneWhole, twoWhole, "a whole float should not become an int in company")
	require.Equal(t, oneBlob, twoBlob, "bytes should not become a base64 string in company")
	require.Equal(t, cue.FloatKind, twoWhole)
	require.Equal(t, cue.BytesKind, twoBlob)
}
