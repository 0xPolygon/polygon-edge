// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package cmemprof

import (
	"runtime"
)

import "C"

//export recordAllocationSample
func recordAllocationSample(size uint) {
	p := activeProfile.Load().(*Profile)
	if p == nil {
		return
	}

	// There are several calls in the call stack we should skip. Ultimately
	// we'd like to get only the actual allocation call, but we can settle
	// for profile_allocation. This call should be skipped, together with
	// any of the C->Go transition functions like cgocallback and the
	// wrapper for this function that's called through the exported version.
	//
	// TODO: how much to skip? Is the value consistent across Go releases?
	// The value below just comes from experimentation with Go 1.17.5
	const skip = 6
	var pcs callStack
	runtime.Callers(skip, pcs[:])
	p.insert(pcs, size)
}
