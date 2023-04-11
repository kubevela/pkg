package kube

#Apply: {
	#do:       "apply"
	#provider: "kube"

	// +usage=The params of this action
	$params: {
		// +usage=The cluster to use
		cluster: *"" | string
		// +usage=The resource to apply
		resource: {...}
		// +usage=The options to apply
		options: {
			// +usage=The strategy to apply the resource
			threeWayMergePatch: {
				// +usage=The strategy to apply the resource
				enabled: *true | bool
				// +usage=The annotation prefix to use for the three way merge patch
				annotationPrefix: *"resource" | string
			}
		}
	}
	// +usage=The result of this action, will be filled with the patched resource after the action is executed
	$returns: {
		...
	}
}

#Get: {
	#do:       "get"
	#provider: "kube"

	// +usage=The params of this action
	$params: {
		// +usage=The cluster to use
		cluster: *"" | string
		// +usage=The resource to get
		resource: {
			// +usage=The api version of the resource
			apiVersion: string
			// +usage=The kind of the resource
			kind: string
			// +usage=The metadata of the resource
			metadata: {
				// +usage=The name of the resource
				name: string
				// +usage=The namespace of the resource
				namespace?: string
			}
		}
	}

	// +usage=The result of this action, will be filled with the resource read from the cluster after the action is executed
	$returns: {
		...
	}
}

#List: {
	#do:       "list"
	#provider: "kube"

	// +usage=The params of this action
	$params: {
		// +usage=The cluster to use
		cluster: *"" | string
		// +usage=The resource to list
		resource: {
			// +usage=The api version of the resource
			apiVersion: string
			// +usage=The kind of the resource
			kind: string
		}
		// +usage=The filter to list the resources
		filter?: {
			// +usage=The namespace to list the resources
			namespace: *"" | string
			// +usage=The label selector to filter the resources
			matchingLabels?: {...}
		}
	}

	// +usage=The result of this action, will be filled with the resource list from the cluster after the action is executed
	$returns: {
		...
	}
}

#Patch: {
	#do:       "patch"
	#provider: "kube"

	// +usage=The params of this action
	$params: {
		// +usage=The cluster to use
		cluster: *"" | string
		// +usage=The resource to patch
		resource: {
			// +usage=The api version of the resource
			apiVersion: string
			// +usage=The kind of the resource
			kind: string
			// +usage=The metadata of the resource
			metadata: {
				// +usage=The name of the resource
				name: string
				// +usage=The namespace of the resource
				namespace?: string
			}
		}
		// +usage=The patch to be applied to the resource with kubernetes patch
		patch: *{
			// +usage=The type of patch being provided
			type: "merge"
			data: {...}
		} | {
			// +usage=The type of patch being provided
			type: "json"
			data: [...{...}]
		} | {
			// +usage=The type of patch being provided
			type: "strategic"
			data: {...}
		}
	}

	// +usage=The result of this action, will be filled with the patched resource after the action is executed
	$returns: {
		...
	}
}
