package kube

Encode: {
	#do:       "encode"
	#provider: "cue"

	// +usage=The params of this action
	$params: {...}

	// +usage=The result of this action
	$returns: string
}

Decode: {
	#do:       "decode"
	#provider: "cue"

	// +usage=The params of this action
	$params: string

	// +usage=The result of this action
	$returns: {...}
}
