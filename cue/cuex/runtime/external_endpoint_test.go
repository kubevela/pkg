package runtime_test

import (
	"context"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/apis/cue/v1alpha1"
	"github.com/kubevela/pkg/cue/cuex/runtime"
)

// TestUnparseableEndpointErrors: the endpoint comes from a Package in the
// cluster, so one that cannot be made into a request has to come back as an
// error. Putting the trace headers on before that is checked dereferences a
// nil request and takes the controller with it.
func TestUnparseableEndpointErrors(t *testing.T) {
	fn := &runtime.ExternalProviderFn{
		Provider: v1alpha1.Provider{
			Protocol: v1alpha1.ProtocolHTTP,
			Endpoint: "http://exa\x7fmple.com",
		},
		Fn: "ask",
	}
	v := cuecontext.New().CompileString(`$params: {a: 1}`)
	require.NotPanics(t, func() {
		_, err := fn.Call(context.Background(), v)
		require.Error(t, err)
	})
}
