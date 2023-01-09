package kube

#Get: {
	#do:       "get"
	#provider: "kube"

	// +usage=The cluster to use
	cluster: *"" | string
	// +usage=The resource to read, this field will be filled with the resource read from the cluster after the action is executed
	value: {
		// +usage=The api version of the resource
		apiVersion: string
		// +usage=The kind of the resource
		kind: string
		metadata: {
			name: string
			namespace?: string
			...
		}
		...
	}
}

#List: {
	#do:       "list"
	#provider: "kube"

	// +usage=The cluster to use
	cluster: *"" | string
	// +usage=The filter to list the resources
	filter?: {
		// +usage=The namespace to list the resources
		namespace?: *"" | string
		// +usage=The label selector to filter the resources
		matchingLabels?: {...}
	}
	// +usage=The listed resources will be filled in this field after the action is executed
	list: {
		// +usage=The api version of the resource
		apiVersion: string
		// +usage=The kind of the resource
		kind: string
		...
	}
}