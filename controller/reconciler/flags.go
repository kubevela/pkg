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

package reconciler

import (
	"github.com/spf13/pflag"
)

// AddFlags add flags for controller reconciles
func AddFlags(set *pflag.FlagSet) {
	AddReconcileTimeoutFlags(set)
}

// AddReconcileTimeoutFlags add flags for controller reconcile timeout
func AddReconcileTimeoutFlags(set *pflag.FlagSet) {
	set.DurationVarP(&ReconcileTimeout,
		"reconcile-timeout", "",
		ReconcileTimeout,
		"the timeout for controller reconcile")
	set.DurationVarP(&ReconcileTerminationGracefulPeriod,
		"reconcile-termination-graceful-period", "",
		ReconcileTerminationGracefulPeriod,
		"graceful period for terminating reconcile")
}
