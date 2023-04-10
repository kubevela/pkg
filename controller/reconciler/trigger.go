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

package reconciler

import (
	"net/http"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const (
	paramsName      = "name"
	paramsNamespace = "namespace"
)

// NewTriggerHandler get name and namespace from query params, add it to the event channel
func NewTriggerHandler(eventChannel chan event.GenericEvent) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		name, ns := request.URL.Query().Get(paramsName), request.URL.Query().Get(paramsNamespace)
		if len(name) == 0 || len(ns) == 0 {
			http.Error(writer, "neither name nor namespace could be empty", http.StatusBadRequest)
			return
		}
		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
		obj.SetName(name)
		obj.SetNamespace(ns)
		eventChannel <- event.GenericEvent{Object: obj}
		writer.WriteHeader(http.StatusOK)
	})
}

// RegisterTriggerHandler register trigger handler to the webhook server of mgr
func RegisterTriggerHandler(mgr controllerruntime.Manager, path string, bufferSize int) chan event.GenericEvent {
	eventChannel := make(chan event.GenericEvent, bufferSize)
	mgr.GetWebhookServer().Register(path, NewTriggerHandler(eventChannel))
	return eventChannel
}
