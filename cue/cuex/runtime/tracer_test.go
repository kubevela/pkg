/*
Copyright 2025 The KubeVela Authors.

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

package runtime

import (
	"context"
	"cuelang.org/go/cue"
	"encoding/json"
	"errors"
	"github.com/kubevela/pkg/apis/cue/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTracerNoBaggage(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NotNil(t, r.Header.Get("X-Vela-Encoded-Context"))
		assert.NotNil(t, r.Header.Get("Traceparent"))

		ctx := ContextFromHeaders(r)
		_, _ = GetPropagatedContext(ctx)

		w.WriteHeader(200)
		_, _ = w.Write([]byte("{ \"response\": true }"))
	}))

	provider := v1alpha1.Provider{
		Protocol: v1alpha1.ProtocolHTTP,
		Endpoint: testServer.URL,
	}

	ctx, span := StartSpan(context.Background(), "span")
	defer span.End()

	_, ok := GetPropagatedContext(ctx)
	assert.False(t, ok)
	assert.True(t, span.SpanContext().SpanID().IsValid())
	assert.True(t, span.SpanContext().TraceID().IsValid())

	fn := ExternalProviderFn{
		Provider: provider,
		Fn:       "do",
	}

	_, err := fn.Call(ctx, cue.Value{})
	require.NoError(t, err)
}

func TestTracerWithBaggage(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NotNil(t, r.Header.Get("X-Vela-Encoded-Context"))
		assert.NotNil(t, r.Header.Get("Traceparent"))

		ctx := ContextFromHeaders(r)

		pCtx, ok := GetPropagatedContext(ctx)
		assert.True(t, ok)

		assert.NotNil(t, pCtx.RawJSON())

		var mapCtx map[string]interface{}
		err := pCtx.UnmarshalContext(&mapCtx)
		assert.NoError(t, err)
		assert.Equal(t, mapCtx["appName"], "app-name")
		assert.Equal(t, mapCtx["namespace"], "a-namespace")

		cueContext, err := pCtx.GetCueContext()
		assert.NoError(t, err)
		appName, err := cueContext.LookupPath(cue.ParsePath("appName")).String()
		assert.NoError(t, err)
		assert.Equal(t, "app-name", appName)
		namespace, err := cueContext.LookupPath(cue.ParsePath("namespace")).String()
		assert.NoError(t, err)
		assert.Equal(t, "a-namespace", namespace)

		tertiary := pCtx.Context.Value("tertiary")
		assert.Equal(t, "value", tertiary)

		span := trace.SpanFromContext(ctx)
		assert.True(t, span.SpanContext().SpanID().IsValid())
		assert.True(t, span.SpanContext().TraceID().IsValid())

		w.WriteHeader(200)
		_, _ = w.Write([]byte("{ \"response\": true }"))
	}))

	provider := v1alpha1.Provider{
		Protocol: v1alpha1.ProtocolHTTP,
		Endpoint: testServer.URL,
	}

	jsonCtx := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"appName":   "app-name",
			"namespace": "a-namespace",
		},
	}

	ctx, span, _ := StartSpanWithBaggage(context.Background(), "span", jsonCtx)
	ctx, _ = WithBaggage(ctx, map[string]string{
		"tertiary": "value",
	})

	defer span.End()

	fn := ExternalProviderFn{
		Provider: provider,
		Fn:       "do",
	}

	_, err := fn.Call(ctx, cue.Value{})
	require.NoError(t, err)
}

type dummyContext struct{}

func (d dummyContext) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"foo": "bar"})
}

func TestStartSpanWithBaggage_MemberError(t *testing.T) {
	original := newBaggageMember
	defer func() { newBaggageMember = original }()

	newBaggageMember = func(k string, v string, props ...baggage.Property) (baggage.Member, error) {
		return baggage.Member{}, errors.New("forced error")
	}

	ctx := context.Background()
	ctx, span, err := StartSpanWithBaggage(ctx, "test-span", dummyContext{})
	assert.Error(t, err)
	assert.NotNil(t, span)
	assert.True(t, span.SpanContext().SpanID().IsValid())
	assert.True(t, span.SpanContext().TraceID().IsValid())

	b := baggage.FromContext(ctx)
	assert.Empty(t, b.Members())
}

func TestStartSpanWithBaggage_BaggageError(t *testing.T) {
	original := newBaggage
	defer func() { newBaggage = original }()

	newBaggage = func(members ...baggage.Member) (baggage.Baggage, error) {
		return baggage.Baggage{}, errors.New("forced error")
	}

	ctx := context.Background()
	ctx, span, err := StartSpanWithBaggage(ctx, "test-span", dummyContext{})
	assert.Error(t, err)
	assert.NotNil(t, span)
	assert.True(t, span.SpanContext().SpanID().IsValid())
	assert.True(t, span.SpanContext().TraceID().IsValid())

	b := baggage.FromContext(ctx)
	assert.Empty(t, b.Members())
}

type jsonMarshalFailure struct{}

func (d jsonMarshalFailure) MarshalJSON() ([]byte, error) {
	return nil, errors.New("failed")
}

func TestStartSpanWithBaggage_MarshalError(t *testing.T) {
	ctx := context.Background()
	ctx, span, err := StartSpanWithBaggage(ctx, "test-span", jsonMarshalFailure{})

	assert.Error(t, err)
	assert.NotNil(t, span)
	assert.True(t, span.SpanContext().SpanID().IsValid())
	assert.True(t, span.SpanContext().TraceID().IsValid())

	b := baggage.FromContext(ctx)
	assert.Empty(t, b.Members())
}

func TestWithBaggage_MemberError(t *testing.T) {
	original := newBaggageMember
	defer func() { newBaggageMember = original }()

	newBaggageMember = func(k string, v string, props ...baggage.Property) (baggage.Member, error) {
		if k == "fail" {
			return baggage.Member{}, errors.New("forced error")
		}
		return original(k, v, props...)
	}

	bag := map[string]string{
		"fail":   "failure",
		"member": "value",
	}

	ctx := context.Background()
	ctx, err := WithBaggage(ctx, bag)
	assert.Error(t, err)

	b := baggage.FromContext(ctx)
	assert.Equal(t, "member", b.Members()[0].Key())
	assert.Equal(t, "value", b.Members()[0].Value())
}

func TestWithBaggage_BaggageError(t *testing.T) {
	original := newBaggage
	defer func() { newBaggage = original }()

	newBaggage = func(members ...baggage.Member) (baggage.Baggage, error) {
		return baggage.Baggage{}, errors.New("forced error")
	}

	bag := map[string]string{
		"member":  "value",
		"another": "value",
	}

	ctx := context.Background()
	ctx, err := WithBaggage(ctx, bag)
	assert.Error(t, err)

	b := baggage.FromContext(ctx)
	assert.Empty(t, b.Members())
}

func TestExtract_InvalidBase64(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://test.com", nil)
	req.Header.Set("X-Vela-Encoded-Context", "invalid base64!")

	ctx := TraceHeaderPropagator{}.Extract(context.Background(), propagation.HeaderCarrier(req.Header))
	pCtx, _ := GetPropagatedContext(ctx)

	assert.Empty(t, pCtx.pCtx)
}

func TestGetCueContext_Invalid(t *testing.T) {
	propagatedCtx := PropagatedCtx{pCtx: []byte("+++")}

	val, err := propagatedCtx.GetCueContext()
	assert.Error(t, err)
	assert.False(t, val.Exists())
}

func TestGetPropagatedContext_WhenEmpty(t *testing.T) {
	pCtx, ok := GetPropagatedContext(context.Background())
	assert.False(t, ok)
	assert.NotNil(t, pCtx)
	assert.Empty(t, pCtx.pCtx)

	cueContext, err := pCtx.GetCueContext()
	assert.NoError(t, err)
	assert.True(t, cueContext.Exists())

	jsonContext := pCtx.RawJSON()
	assert.Empty(t, jsonContext)
}
