package debug

#Noop: {
	#do:       "noop"
	#provider: "debug"

	// +usage=The params of this action
	$params: {
		// +usage=Named in the log line, so one call can be told from another
		tag?: string
	}
	// +usage=What was passed in, so a call can be chained onto another
	$returns?: {
		tag: string
		...
	}
	...
}
