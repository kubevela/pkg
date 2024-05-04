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

package compression

import "go.uber.org/automaxprocs/maxprocs"

// Type the compression type
type Type string

const (
	// Uncompressed does not compress data. Note that you should NOT actually Uncompressed
	// Type. We do not provide a compressor for Uncompressed Type.
	// It is just for compatibility purposes.
	Uncompressed Type = ""
	// Gzip compresses data using gzip
	Gzip Type = "gzip"
	// Zstd compresses data using zstd
	Zstd Type = "zstd"
)

var compressors = make(map[Type]compressor)

type compressor interface {
	// compress marshals the obj using JSON, then compresses it.
	compress(obj interface{}) ([]byte, error)
	// decompress decompresses the data, then unmarshalls it using JSON.
	decompress(compressed []byte, obj interface{}) error
	init()
}

func init() {
	// init automaxprocs to disable its default stdout logger
	maxprocs.Set()
	// Add compressors
	compressors[Gzip] = &gzipCompressor{}
	compressors[Zstd] = &zstdCompressor{}

	// init compressors
	for _, c := range compressors {
		c.init()
	}
}
