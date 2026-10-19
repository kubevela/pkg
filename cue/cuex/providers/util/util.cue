// Copyright 2026 The KubeVela Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package util

#Truncate: {
	#do:       "truncate"
	#provider: "util"

	$params: {
		// +usage=The raw desired value
		value: string
		// +usage=The hard length cap (counted in runes), applied to the whole result
		maxLength: int
		// +usage=Delimiter joining the prefix, truncated base, and hash suffix
		delimiter: *"-" | string
		// +usage=Number of hex chars in the uniqueness suffix, at most 64 (the width of a sha256 digest)
		hashLength: *8 | int & <=64
		// +usage=Prefix always preserved verbatim and counted against maxLength
		prefix: *"" | string
		// +usage=Trim the base back to the last delimiter so it never ends in a partial segment
		preserveSegments: *false | bool
	}

	// +usage=The result of this action, filled in after the action is executed
	$returns: {
		// +usage=The value, guaranteed <= maxLength runes
		value?: string
		...
	}
}

#TimeParse: {
	#do:       "timeparse"
	#provider: "util"

	$params: {
		// +usage=The timestamp to parse
		value: string
		// +usage=Go reference layout to parse with, empty means RFC3339
		layout: *"" | string
	}

	// +usage=The result of this action, filled in after the action is executed
	$returns: {
		// +usage=Whole seconds since the Unix epoch
		unix?: int
		// +usage=Nanoseconds since the Unix epoch
		unixNano?: int
		// +usage=The parsed instant in RFC3339Nano layout, preserving the input zone offset
		rfc3339?: string
		...
	}
}

#TimeAdd: {
	#do:       "timeadd"
	#provider: "util"

	$params: {
		// +usage=The timestamp to offset
		value: string
		// +usage=Go duration string such as "720h", "-30m" or "1h30m"
		duration: string
		// +usage=Go reference layout used to both parse and emit, empty means RFC3339
		layout: *"" | string
	}

	// +usage=The result of this action, filled in after the action is executed
	$returns: {
		// +usage=The offset timestamp, in the same layout and zone offset as the input
		value?: string
		...
	}
}

#TimeDiff: {
	#do:       "timediff"
	#provider: "util"

	$params: {
		// +usage=The earlier timestamp, subtracted from to
		from: string
		// +usage=The later timestamp
		to: string
		// +usage=Go reference layout used to parse both, empty means RFC3339
		layout: *"" | string
	}

	// +usage=The result of this action, filled in after the action is executed
	$returns: {
		// +usage=The interval as a Go duration string, negative when to precedes from
		duration?: string
		// +usage=The same interval in nanoseconds
		nanoseconds?: int
		...
	}
}

#TimeCompare: {
	#do:       "timecompare"
	#provider: "util"

	$params: {
		// +usage=The left-hand timestamp
		a: string
		// +usage=The right-hand timestamp
		b: string
		// +usage=Go reference layout used to parse both, empty means RFC3339
		layout: *"" | string
	}

	// +usage=The result of this action, filled in after the action is executed
	$returns: {
		// +usage=-1 if a is before b, 0 if the same instant, +1 if a is after b
		result?: int
		...
	}
}
