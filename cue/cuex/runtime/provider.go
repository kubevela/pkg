/*
Copyright 2022 The KubeVela Authors.

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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel/propagation"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/pkg/apis/cue/v1alpha1"
	"github.com/kubevela/pkg/cue/cuex/providers"
	"github.com/kubevela/pkg/util/singleton"
)

// ConcurrentProviderFn is implemented by a provider function that may run
// alongside its other calls, by Concurrent wrapping one that can be split.
//
// The split is what makes it possible. CUE values built by one context are not
// safe to read concurrently, so the reading and the work have to happen at
// different times: the resolver decodes each call's parameters one at a time,
// then runs the functions together on what it decoded.
type ConcurrentProviderFn interface {
	ProviderFn
	// Decode reads what the call needs out of its value. It touches CUE, so
	// the resolver calls it for one call at a time.
	Decode(value cue.Value) (any, error)
	// Invoke runs the function on what Decode read. It must touch no CUE
	// value, no state shared with its other calls, and nothing outside the
	// process that depends on the order calls arrive in.
	Invoke(ctx context.Context, decoded any) (any, error)
}

// splittable is the half of a provider function that can be separated from the
// value it reads. GenericProviderFn and ExternalProviderFn can; a
// NativeProviderFn is handed a cue.Value and reads it throughout, so there is
// nowhere to put the seam.
type splittable interface {
	decode(value cue.Value) (any, error)
	invoke(ctx context.Context, decoded any) (any, error)
}

// Concurrent marks a provider function as safe to run alongside its other
// calls. It is only ever run that way if a template asks as well, with
// @concurrency.
//
// Marking a function that cannot be split - a NativeProviderFn - returns it
// unchanged, and it goes on running one call at a time.
func Concurrent(fn ProviderFn) ProviderFn {
	if parts, ok := fn.(splittable); ok {
		return concurrentFn{ProviderFn: fn, splittable: parts}
	}
	return fn
}

type concurrentFn struct {
	ProviderFn
	splittable
}

// Decode .
func (in concurrentFn) Decode(value cue.Value) (any, error) {
	return in.splittable.decode(value)
}

// Invoke .
func (in concurrentFn) Invoke(ctx context.Context, decoded any) (any, error) {
	return in.splittable.invoke(ctx, decoded)
}

// CallForResult passes through, so marking a function does not cost it the
// result form.
func (in concurrentFn) CallForResult(ctx context.Context, value cue.Value) (any, error) {
	decoded, err := in.splittable.decode(value)
	if err != nil {
		return nil, err
	}
	return in.splittable.invoke(ctx, decoded)
}

var (
	_ ConcurrentProviderFn = concurrentFn{}
	_ ResultProviderFn     = concurrentFn{}
)

// ResultProviderFn is optionally implemented by a ProviderFn that can hand
// back its result as a Go value.
//
// A ProviderFn has to return a cue.Value, so it unifies its result into the
// call's node - which the resolver then unifies into the value the node came
// from. Returning the result instead leaves one unification where there were
// two, and a fill costs more than the function bodies do.
type ResultProviderFn interface {
	ProviderFn
	// CallForResult runs the function and returns its result. The result is
	// written at the call's own path, so it is shaped like the node: a
	// $returns field rather than a bare value.
	CallForResult(ctx context.Context, value cue.Value) (any, error)
}

var (
	_ ProviderFn       = GenericProviderFn[any, any](nil)
	_ ResultProviderFn = GenericProviderFn[any, any](nil)
	_ splittable       = GenericProviderFn[any, any](nil)

	paramsPath     = cue.ParsePath(providers.ParamsKey)
	wholeValuePath = cue.ParsePath("")
)

// GenericProviderFn generic function that implements ProviderFn interface
type GenericProviderFn[T any, U any] func(context.Context, *T) (*U, error)

// Call marshal value into json and decode into underlying function input
// parameters, then fill back the returned output value
func (fn GenericProviderFn[T, U]) Call(ctx context.Context, value cue.Value) (cue.Value, error) {
	ret, err := fn.CallForResult(ctx, value)
	if err != nil {
		return value, err
	}
	return value.FillPath(wholeValuePath, ret), nil
}

// CallForResult .
func (fn GenericProviderFn[T, U]) CallForResult(ctx context.Context, value cue.Value) (any, error) {
	decoded, err := fn.decode(value)
	if err != nil {
		return nil, err
	}
	return fn.invoke(ctx, decoded)
}

func (fn GenericProviderFn[T, U]) decode(value cue.Value) (any, error) {
	return decodeParams[T](value)
}

func (fn GenericProviderFn[T, U]) invoke(ctx context.Context, decoded any) (any, error) {
	params, ok := decoded.(*T)
	if !ok {
		return nil, fmt.Errorf("provider function given %T, want %T", decoded, params)
	}
	return fn(ctx, params)
}

// decodeParams reads a call's parameters into the function's input type.
//
// Only $params is marshalled where the input type asks for nothing else - the
// rest of the node is the call's own bookkeeping, the provider name, the
// function name, the shape of the result, and marshalling it as well costs an
// order of magnitude more. An input type that declares other fields is filled
// from the whole node as it always was, so a provider outside this repo that
// reads something beside its parameters keeps getting it rather than silently
// getting nothing. A call with no $params is marshalled whole for the same
// reason: there is nowhere else its input could come from.
func decodeParams[T any](value cue.Value) (*T, error) {
	var (
		bs  []byte
		err error
	)
	params := value.LookupPath(paramsPath)
	if params.Exists() && paramsAreTheWholeInput[T]() {
		if bs, err = params.MarshalJSON(); err != nil {
			return nil, err
		}
		bs = wrapParams(bs)
	} else if bs, err = value.MarshalJSON(); err != nil {
		return nil, err
	}
	decoded := new(T)
	if err = json.Unmarshal(bs, decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// inputIsParamsOnly remembers, per input type, whether $params covers all of
// it. A provider function is registered once and called many times.
var inputIsParamsOnly sync.Map

// paramsAreTheWholeInput reports whether every field the input type declares
// is the parameters. Anything less certain - another field, an embedded type,
// something that is not a struct - is answered no, and the caller then reads
// the whole node.
func paramsAreTheWholeInput[T any]() bool {
	typ := reflect.TypeOf((*T)(nil)).Elem()
	if known, ok := inputIsParamsOnly.Load(typ); ok {
		return known.(bool)
	}
	only := typ.Kind() == reflect.Struct
	for i := 0; only && i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Anonymous {
			// an embedded type promotes its exported fields into the JSON,
			// including one that is itself unexported, so this is checked
			// before the unexported skip below rather than after it
			only = false
			break
		}
		if field.PkgPath != "" {
			continue // unexported, so json never sees it
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		switch name {
		case "-":
			continue
		case providers.ParamsKey:
		default:
			only = false
		}
	}
	inputIsParamsOnly.Store(typ, only)
	return only
}

// wrapParams puts marshalled parameters back under the key the input type
// declares them with, so the type itself does not have to change.
func wrapParams(params []byte) []byte {
	prefix := `{"` + providers.ParamsKey + `":`
	out := make([]byte, 0, len(prefix)+len(params)+1)
	out = append(out, prefix...)
	out = append(out, params...)
	return append(out, '}')
}

var (
	_ ProviderFn       = (*ExternalProviderFn)(nil)
	_ ResultProviderFn = (*ExternalProviderFn)(nil)
	_ splittable       = (*ExternalProviderFn)(nil)
)

// ExternalProviderFn external provider that implements ProviderFn interface
type ExternalProviderFn struct {
	v1alpha1.Provider
	Fn string
}

// DefaultClientInsecureSkipVerify set if the default external provider client
// use insecure-skip-verify
var DefaultClientInsecureSkipVerify = true

// DefaultClient client for dealing requests
var DefaultClient = singleton.NewSingleton(func() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: DefaultClientInsecureSkipVerify},
		},
	}
})

// Call dial external endpoints by passing the json data of the input parameter,
// then fill back returned values
func (in *ExternalProviderFn) Call(ctx context.Context, value cue.Value) (cue.Value, error) {
	ret, err := in.CallForResult(ctx, value)
	if err != nil {
		return value, err
	}
	return value.FillPath(wholeValuePath, ret), nil
}

// CallForResult .
func (in *ExternalProviderFn) CallForResult(ctx context.Context, value cue.Value) (any, error) {
	decoded, err := in.decode(value)
	if err != nil {
		return nil, err
	}
	return in.invoke(ctx, decoded)
}

// decode reads the call's parameters out as JSON, which is what goes on the
// wire. This is the only part that touches CUE.
func (in *ExternalProviderFn) decode(value cue.Value) (any, error) {
	return value.LookupPath(paramsPath).MarshalJSON()
}

// invoke dials the endpoint. It holds nothing of the value it came from, so
// calls to it can be in flight together.
func (in *ExternalProviderFn) invoke(ctx context.Context, decoded any) (any, error) {
	bs, ok := decoded.([]byte)
	if !ok {
		return nil, fmt.Errorf("external provider given %T, want the encoded parameters", decoded)
	}
	switch in.Protocol {
	case v1alpha1.ProtocolHTTP, v1alpha1.ProtocolHTTPS:
		ep := fmt.Sprintf("%s/%s", strings.TrimSuffix(in.Endpoint, "/"), in.Fn)
		req, err := http.NewRequest(http.MethodPost, ep, bytes.NewReader(bs))
		if err != nil {
			// before the headers go on: an endpoint that cannot be parsed
			// leaves req nil, and a Package carries one from the cluster
			return nil, err
		}
		in.InjectHeaders(ctx, req)
		req.Header.Set("Content-Type", runtime.ContentTypeJSON)
		for k, v := range in.Header {
			req.Header.Set(k, v)
		}
		resp, err := DefaultClient.Get().Do(req.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if bs, err = io.ReadAll(resp.Body); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("protocol %s not supported yet", in.Protocol)
	}
	ret := &map[string]any{}
	if err := json.Unmarshal(bs, ret); err != nil {
		return nil, err
	}
	return map[string]any{providers.ReturnsKey: ret}, nil
}

// InjectHeaders Injects headers from the current span into the http request headers
func (in *ExternalProviderFn) InjectHeaders(ctx context.Context, r *http.Request) {
	TraceHeaderPropagator{}.Inject(ctx, propagation.HeaderCarrier(r.Header))
}

var _ ProviderFn = NativeProviderFn(nil)

// NativeProviderFn native function that implements ProviderFn interface
type NativeProviderFn func(context.Context, cue.Value) (cue.Value, error)

// Call .
func (fn NativeProviderFn) Call(ctx context.Context, value cue.Value) (cue.Value, error) {
	return fn(ctx, value)
}
