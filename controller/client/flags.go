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

package client

import "github.com/spf13/pflag"

// AddFlags add flags for default controller client
func AddFlags(set *pflag.FlagSet) {
	AddTimeoutControllerClientFlags(set)
}

// AddTimeoutControllerClientFlags add flags for default timeout controller client
func AddTimeoutControllerClientFlags(set *pflag.FlagSet) {
	set.DurationVarP(&DefaultTimeoutClientOptions.RequestTimeout,
		"controller-client-request-timeout", "",
		DefaultTimeoutClientOptions.RequestTimeout,
		"The timeout value for controller client requests.")
	set.DurationVarP(&DefaultTimeoutClientOptions.LongRunningRequestTimeout,
		"controller-client-long-running-request-timeout", "",
		DefaultTimeoutClientOptions.LongRunningRequestTimeout,
		"The timeout value for controller client long-running (list) requests.")
	set.DurationVarP(&DefaultTimeoutClientOptions.MutatingRequestTimeout,
		"controller-client-mutating-request-timeout", "",
		DefaultTimeoutClientOptions.MutatingRequestTimeout,
		"The timeout value for controller client mutating (update, patch, delete) requests.")
}
