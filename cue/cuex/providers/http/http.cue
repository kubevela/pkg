package http

#Do: {
	#do:       "do"
	#provider: "http"

	// +usage=The params of this action
	$params: {
		// +usage=The method of HTTP request
		method: *"GET" | "POST" | "PUT" | "DELETE" | "PATCH" | "CONNECT" | "OPTIONS" | "TRACE"
		// +usage=The url to request
		url: string
		// +usage=The request config
		request?: {
			// +usage=The request body
			body?: string
			// +usage=The header of the request
			header?: [string]: string
			// +usage=The trailer of the request
			trailer?: [string]: string
			...
		}
	}
	// +usage=The response of the request will be filled in this field after the action is executed
	$returns: {
		// +usage=The body of the response
		body?: string
		// +usage=The header of the response
		header?: [string]: [...string]
		// +usage=The trailer of the response
		trailer?: [string]: [...string]
		// +usage=The status code of the response
		statusCode?: int
		...
	}
	...
}

// A GET changes nothing at the far end, so several may be in flight at once
// where a template asks with @concurrency. The rest keep to one at a time: the
// resolver would otherwise be free to reorder a POST against a DELETE, and
// only the caller knows whether that is safe.
#Get: #Do & {$params: method: "GET", #config: maxPerRender: 16}

#Post: #Do & {$params: method: "POST"}

#Put: #Do & {$params: method: "PUT"}

#Patch: #Do & {$params: method: "PATCH"}

#Delete: #Do & {$params: method: "DELETE"}

#Connect: #Do & {$params: method: "CONNECT"}

#OPTIONS: #Do & {$params: method: "OPTIONS"}

#TRACE: #Do & {$params: method: "TRACE"}
