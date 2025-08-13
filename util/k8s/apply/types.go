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

package apply

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/k8s/patch"
)

// UpdateStrategy the strategy for updating
type UpdateStrategy int

const (
	// Patch use the three-way-merge-patch to update the target resource
	Patch UpdateStrategy = iota
	// Replace directly replacing the whole target resource
	Replace
	// Recreate delete the resource first and create it
	Recreate
	// Skip do not make update to the target resource
	Skip
)

// Options interface for doing Apply
type Options interface {
	// DryRun whether the current apply is a dry-run operation
	DryRun() DryRunOption
	// GetUpdateStrategy decide how the target object should be updated
	GetUpdateStrategy(existing, desired client.Object) (UpdateStrategy, error)
}

// PatchActionProvider if the given action implement this interface, the PatchAction
// will be used during three-way-merge-patch
type PatchActionProvider interface {
	GetPatchAction() patch.PatchAction
}

// PreApplyHook run before creating/updating the object, could be used to make
// validation or mutation
type PreApplyHook interface {
	PreApply(desired client.Object) error
}

// PreCreateHook run before creating the object, could be used to make validation
// or mutation
type PreCreateHook interface {
	PreCreate(desired client.Object) error
}

// PreUpdateHook run before updating the object, could be used to propagating
// existing configuration, make mutation to desired object or validate it
type PreUpdateHook interface {
	PreUpdate(existing, desired client.Object) error
}

// DryRunOption a bool option for client.DryRunAll
type DryRunOption bool

// ApplyToCreate implements client.CreateOption
func (in DryRunOption) ApplyToCreate(options *client.CreateOptions) {
	if in {
		client.DryRunAll.ApplyToCreate(options)
	}
}

// ApplyToUpdate implements client.UpdateOption
func (in DryRunOption) ApplyToUpdate(options *client.UpdateOptions) {
	if in {
		client.DryRunAll.ApplyToUpdate(options)
	}
}

// ApplyToPatch implements client.PatchOption
func (in DryRunOption) ApplyToPatch(options *client.PatchOptions) {
	if in {
		client.DryRunAll.ApplyToPatch(options)
	}
}
