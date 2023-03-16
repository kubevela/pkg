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

package profiling

import (
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"runtime"

	"k8s.io/klog/v2"

	"github.com/kubevela/pkg/util/errhandler"
)

// NewProfilingHandler create a profiling handler
func NewProfilingHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/mem/stat", func(writer http.ResponseWriter, request *http.Request) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		bs, _ := json.Marshal(ms)
		_, _ = writer.Write(bs)
	})
	mux.HandleFunc("/gc", func(writer http.ResponseWriter, request *http.Request) {
		runtime.GC()
	})
	return mux
}

// StartProfilingServer listen to the pprofAddr and export the profiling results
// If the errChan is nil, this function will panic when the listening error occurred.
func StartProfilingServer(errChan chan error) {
	if Addr == "" {
		return
	}
	klog.Infof("start profiling server at %s", Addr)
	err := http.ListenAndServe(Addr, NewProfilingHandler())
	errhandler.NotifyOrPanic(errChan)(err)
}
