/*
Copyright 2023 The KubeVela Authors.

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

package util

import (
	"strings"

	"cuelang.org/go/cue"
	"sigs.k8s.io/yaml"
)

// PrintFormat format for printing cue.Value
type PrintFormat string

const (
	// PrintFormatJson json
	PrintFormatJson PrintFormat = "json"
	// PrintFormatYaml yaml
	PrintFormatYaml PrintFormat = "yaml"
	// PrintFormatCue cue
	PrintFormatCue PrintFormat = "cue"
)

// PrintConfig config for printing value
type PrintConfig struct {
	format PrintFormat
	path   *cue.Path
}

// PrintOption options for printing value
type PrintOption interface {
	ApplyTo(*PrintConfig)
}

// WithFormat set format for the print value
type WithFormat string

// ApplyTo .
func (in WithFormat) ApplyTo(cfg *PrintConfig) {
	cfg.format = PrintFormat(in)
}

// WithPath set path for the print value
type WithPath string

// ApplyTo .
func (in WithPath) ApplyTo(cfg *PrintConfig) {
	p := strings.TrimSpace(string(in))
	if len(p) > 0 {
		path := cue.ParsePath(p)
		cfg.path = &path
	}
}

// NewPrintConfig create print config
func NewPrintConfig(options ...PrintOption) *PrintConfig {
	cfg := &PrintConfig{format: PrintFormatCue}
	for _, op := range options {
		op.ApplyTo(cfg)
	}
	return cfg
}

// Print print cue.Value
func Print(value cue.Value, options ...PrintOption) ([]byte, error) {
	cfg := NewPrintConfig(options...)
	if cfg.path != nil {
		value = value.LookupPath(*cfg.path)
	}
	switch cfg.format {
	case PrintFormatJson:
		return value.MarshalJSON()
	case PrintFormatYaml:
		bs, err := value.MarshalJSON()
		if err != nil {
			return nil, err
		}
		return yaml.JSONToYAML(bs)
	default:
		s, err := ToString(value)
		return []byte(s), err
	}
}
