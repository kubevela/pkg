package runtime

import (
	"context"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog"
	"net/http"
	"strings"
)

var newBaggageMember = baggage.NewMember // function injection for tests
var newBaggage = baggage.New             // function injection for tests

func init() {
	// set the default OTel implementation if the controller did not already handle the setup
	// fallback functionality - preferred to update the controller to handle this
	oTelTracerProviderSet := otel.GetTracerProvider().Tracer("check") != otel.Tracer("check")
	if !oTelTracerProviderSet {
		tp := tracesdk.NewTracerProvider()
		otel.SetTracerProvider(tp)
	}
}

type ctxKey string

// PropagatedCtx .
type PropagatedCtx struct {
	pCtx []byte
	context.Context
}

const (
	key          ctxKey = "PropagatedCtx"
	headerPrefix        = "X-Vela"
	traceParent         = "traceparent"
	traceState          = "tracestate"
	encodedCtx          = "Encoded-Context"
)

// StartSpan creates a new OpenTelemetry span and returns the updated context and span.
func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	tracer := otel.Tracer(name)
	ctx, span := tracer.Start(ctx, name)
	return ctx, span
}

// StartSpanWithBaggage creates a new OpenTelemetry span and attaches encoded JSON context as baggage.
func StartSpanWithBaggage(ctx context.Context, name string, hCtx json.Marshaler) (context.Context, trace.Span, error) {
	jsonCtx, err := json.Marshal(hCtx)
	if err != nil {
		klog.Errorf("failed to marshal json: %v", err)
		ctx, span := StartSpan(ctx, name)
		return ctx, span, err
	}

	member, err := newBaggageMember(strings.ToLower(encodedCtx), base64.StdEncoding.EncodeToString(jsonCtx))
	if err != nil {
		klog.Warningf("failed to encode context baggage header: \n%v", err)
	} else {
		bag, err := newBaggage(member)
		if err != nil {
			klog.Errorf("failed to create context bag: \n%v", err)
			ctx, span := StartSpan(ctx, name)
			return ctx, span, err
		}
		ctx = baggage.ContextWithBaggage(ctx, bag)
	}

	ctx, span := StartSpan(ctx, name)
	return ctx, span, err
}

// WithBaggage appends additional baggage entries to the context.
func WithBaggage(ctx context.Context, b map[string]string) (context.Context, error) {
	members := baggage.FromContext(ctx).Members()
	var failures []string
	for k, v := range b {
		m, err := newBaggageMember(k, v)
		if err != nil {
			klog.Errorf("failed to create new member\n%v", err)
			failures = append(failures, fmt.Sprintf("failed to create new member %s = %s", k, v))
			continue
		}
		members = append(members, m)
	}
	bag, err := newBaggage(members...)
	if err != nil {
		klog.Errorf("failed to create context bag: \n%v", err)
		return ctx, err
	}

	if len(failures) > 0 {
		klog.Warningf("Baggage created with failed members!")
		return baggage.ContextWithBaggage(ctx, bag), errors.New(strings.Join(failures, "\n"))
	}
	return baggage.ContextWithBaggage(ctx, bag), nil
}

// TraceHeaderPropagator .
type TraceHeaderPropagator struct {
	traceContextPropagator propagation.TraceContext
}

// Inject serializes the span baggage and trace context into the provided HTTP headers.
func (cp TraceHeaderPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	bag := baggage.FromContext(ctx)
	for _, member := range bag.Members() {
		headerName := fmt.Sprintf("%s-%s", headerPrefix, member.Key())
		carrier.Set(headerName, member.Value())
	}
	cp.traceContextPropagator.Inject(ctx, carrier)
}

// Extract reconstructs the span context and baggage from the provided HTTP headers.
func (cp TraceHeaderPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	ctx = TraceHeaderPropagator{}.traceContextPropagator.Extract(ctx, carrier)
	appContext := &PropagatedCtx{
		Context: ctx,
	}
	for _, hKey := range carrier.Keys() {
		value := carrier.Get(hKey)
		if strings.HasPrefix(hKey, headerPrefix) {
			strippedKey := strings.TrimPrefix(hKey, headerPrefix+"-")

			switch strippedKey {

			case encodedCtx:
				dataBytes, err := base64.StdEncoding.DecodeString(value)
				if err != nil {
					klog.Errorf("error decoding base64 context: %v\n", err)
					continue
				}
				appContext.pCtx = dataBytes

			case traceParent, traceState: // do nothing

			default:
				appContext.Context = context.WithValue(appContext.Context, strings.ToLower(strippedKey), value)
			}
		}
	}

	return context.WithValue(ctx, key, appContext)
}

// RawJSON returns the embedded JSON context as a json.RawMessage.
func (p *PropagatedCtx) RawJSON() json.RawMessage {
	return json.RawMessage(p.pCtx)
}

// UnmarshalContext unmarshals the embedded JSON context into the provided struct.
func (p *PropagatedCtx) UnmarshalContext(out interface{}) error {
	return json.Unmarshal(p.pCtx, out)
}

// GetCueContext returns the embedded JSON context as a CUE value.
func (p *PropagatedCtx) GetCueContext() (*cue.Value, error) {
	cueCtx := cuecontext.New()
	cueVal := cueCtx.CompileString(string(p.pCtx))
	if cueVal.Err() != nil {
		return &cue.Value{}, cueVal.Err()
	}
	return &cueVal, nil
}

// ContextFromHeaders extracts the span context and baggage from an HTTP request and returns the reconstructed ctx.
func ContextFromHeaders(r *http.Request) context.Context {
	return TraceHeaderPropagator{}.Extract(context.Background(), propagation.HeaderCarrier(r.Header))
}

// GetPropagatedContext retrieves the PropagatedCtx from the given context if present.
func GetPropagatedContext(ctx context.Context) (*PropagatedCtx, bool) {
	pCtx := ctx.Value(key)
	if pCtx != nil {
		if pc, ok := pCtx.(*PropagatedCtx); ok {
			return pc, true
		}
		return &PropagatedCtx{pCtx: []byte("")}, false
	}
	return &PropagatedCtx{pCtx: []byte("")}, false
}

// Fields returns the list of HTTP headers used for trace context propagation.
func (cp TraceHeaderPropagator) Fields() []string {
	return []string{
		traceParent,
		traceState,
		fmt.Sprintf("%s-%s", headerPrefix, encodedCtx),
	}
}
