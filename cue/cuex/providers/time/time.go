/*
Copyright 2021. The KubeVela Authors.

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

package time

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
)

// TimestampVars .
type TimestampVars struct {
	Date   string `json:"date"`
	Layout string `json:"layout,omitempty"`
}

// TimestampReturnVars .
type TimestampReturnVars struct {
	Timestamp int64 `json:"timestamp"`
}

// TimestampParams .
type TimestampParams = providers.Params[TimestampVars]

// TimestampReturns .
type TimestampReturns = providers.Returns[TimestampReturnVars]

// Timestamp convert date to timestamp
func Timestamp(_ context.Context, params *TimestampParams) (*TimestampReturns, error) {
	date := params.Params.Date
	layout := params.Params.Layout
	if date == "" {
		return nil, fmt.Errorf("empty date to convert")
	}
	if layout == "" {
		layout = time.RFC3339
	}
	t, err := time.Parse(layout, date)
	if err != nil {
		return nil, err
	}
	return &TimestampReturns{
		Returns: TimestampReturnVars{
			Timestamp: t.Unix(),
		},
	}, nil
}

// DateVars .
type DateVars struct {
	Timestamp int64  `json:"timestamp"`
	Layout    string `json:"layout,omitempty"`
}

// DateReturnVars .
type DateReturnVars struct {
	Date string `json:"date"`
}

// DateParams .
type DateParams = providers.Params[DateVars]

// DateReturns .
type DateReturns = providers.Returns[DateReturnVars]

// Date convert timestamp to date
func Date(_ context.Context, params *DateParams) (*DateReturns, error) {
	timestamp := params.Params.Timestamp
	layout := params.Params.Layout
	if layout == "" {
		layout = time.RFC3339
	}
	t := time.Unix(timestamp, 0)
	return &DateReturns{
		Returns: DateReturnVars{
			Date: t.UTC().Format(layout),
		},
	}, nil
}

// ProviderName .
const ProviderName = "time"

//go:embed time.cue
var template string

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"timestamp": cuexruntime.GenericProviderFn[TimestampParams, TimestampReturns](Timestamp),
	"date":      cuexruntime.GenericProviderFn[DateParams, DateReturns](Date),
}))

// GetTemplate return the template
func GetTemplate() string {
	return template
}

// GetProviders return the provider
func GetProviders() map[string]cuexruntime.ProviderFn {
	return map[string]cuexruntime.ProviderFn{
		"timestamp": cuexruntime.GenericProviderFn[TimestampParams, TimestampReturns](Timestamp),
		"date":      cuexruntime.GenericProviderFn[DateParams, DateReturns](Date),
	}
}
