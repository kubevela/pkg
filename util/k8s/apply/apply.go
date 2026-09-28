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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/kubevela/pkg/util/k8s/patch"
)

// Apply try to get desired object and update it, if not exist, create it
// The procedure of Apply is
// 1. Get the desired object
// 2. Run the PreApplyHook
// 3. If not exist, run the PreCreateHook and create the target object, exit
// 4. If exists, run the PreUpdateHook, get the UpdateStrategy, decide how to update
// 5. If Patch, get patch.PatchAction (implemented by PatchActionProvider)
// 6. Do the update operation
// The above procedure will also be affected by the DryRunOption
func Apply(ctx context.Context, c client.Client, desired client.Object, opts Options) error {
	// wrap client for apply spec & status
	c = &Client{c}

	// pre-fill types
	if desired.GetObjectKind().GroupVersionKind().Kind == "" {
		if gvk, err := apiutil.GVKForObject(desired, c.Scheme()); err == nil {
			desired.GetObjectKind().SetGroupVersionKind(gvk)
		}
	}

	// get existing
	existing, err := get(ctx, c, desired)
	if err != nil {
		return fmt.Errorf("cannot get object: %w", err)
	}

	if hook, ok := opts.(PreApplyHook); ok {
		if err = hook.PreApply(desired); err != nil {
			return err
		}
	}

	// create
	if existing == nil {
		if err = create(ctx, c, desired, opts); err != nil {
			return fmt.Errorf("cannot create object: %w", err)
		}
		return nil
	}

	// update
	if err = update(ctx, c, existing, desired, opts); err != nil {
		return fmt.Errorf("cannot update object: %w", err)
	}
	return nil
}

func get(ctx context.Context, c client.Client, desired client.Object) (client.Object, error) {
	o := &unstructured.Unstructured{}
	o.SetGroupVersionKind(desired.GetObjectKind().GroupVersionKind())
	if err := c.Get(ctx, client.ObjectKeyFromObject(desired), o); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return o, nil
}

func create(ctx context.Context, c client.Client, desired client.Object, act Options) error {
	if hook, ok := act.(PreCreateHook); ok {
		if err := hook.PreCreate(desired); err != nil {
			return err
		}
	}
	klog.V(4).InfoS("creating object", "resource", klog.KObj(desired))
	return c.Create(ctx, desired, act.DryRun())
}

func update(ctx context.Context, c client.Client, existing client.Object, desired client.Object, act Options) error {
	if hook, ok := act.(PreUpdateHook); ok {
		if err := hook.PreUpdate(existing, desired); err != nil {
			return err
		}
	}
	strategy, err := act.GetUpdateStrategy(existing, desired)
	if err != nil {
		return err
	}
	switch strategy {
	case Recreate:
		klog.V(4).InfoS("recreating object", "resource", klog.KObj(desired))
		if act.DryRun() { // recreate does not support dryrun
			return nil
		}
		if existing.GetDeletionTimestamp() == nil {
			if err := c.Delete(ctx, existing); err != nil {
				return fmt.Errorf("failed to delete object: %w", err)
			}
		}
		return c.Create(ctx, desired)
	case Replace:
		klog.V(4).InfoS("replacing object", "resource", klog.KObj(desired))
		desired.SetResourceVersion(existing.GetResourceVersion())
		return c.Update(ctx, desired, act.DryRun())
	case Patch:
		klog.V(4).InfoS("patching object", "resource", klog.KObj(desired))
		patchAction := patch.PatchAction{}
		if prd, ok := act.(PatchActionProvider); ok {
			patchAction = prd.GetPatchAction()
		}
		pat, err := patch.ThreeWayMergePatch(existing, desired, &patchAction)
		if err != nil {
			return fmt.Errorf("cannot calculate patch by computing a three way diff: %w", err)
		}
		if isEmptyPatch(pat) {
			return nil
		}
		return c.Patch(ctx, desired, pat, act.DryRun())
	case Skip:
		return nil
	default:
		return fmt.Errorf("unrecognizable update strategy: %v", strategy)
	}
}

func isEmptyPatch(patch client.Patch) bool {
	if patch == nil {
		return true
	}
	data, _ := patch.Data(nil)
	return data == nil || string(data) == "{}"
}
