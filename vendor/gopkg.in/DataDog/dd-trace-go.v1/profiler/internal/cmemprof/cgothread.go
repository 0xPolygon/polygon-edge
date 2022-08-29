// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package cmemprof

// We do the following Go linker trickery to work around the following issue:
//
//	* We are wrapping malloc to profile it
//	* Our wrapper calls from C into Go when an allocation is sampled
//	* Calling from C into Go requires that the M associated with the
//	  current goroutine g0 (in thread local storage) has a real G to
//	  run the Go code.
//	* The x_cgo_thread_start function, used in cgo programs to create OS
//	  threads as part of initializing a new M, calls malloc. pthread
//	  internal functions might also call malloc.
//	* If we happen to profile this call to malloc, the newly created M
//	  has no G to run the Go code. This causes runtime.cgocallback to
//	  access a null pointer (the address of the "current" G) and crash.
//
// Fortunately, the cgo thread creation function is a variable that we can
// manipulate (declared in runtime/cgo/callbacks.go). So we use the go:linkname
// directive to create our own reference to the unexported variable and override
// it with our own wrapper function that sets a flag indicating that we are in
// thread creation and should not profile.

/*
extern void cmemprof_set_cgo_thread_start(void *);
extern void cmemprof_cgo_thread_start(void *);
*/
import "C"

import (
	"unsafe"
)

//go:linkname cgoThreadStart _cgo_thread_start

var (
	cgoThreadStart unsafe.Pointer
)

func init() {
	C.cmemprof_set_cgo_thread_start(cgoThreadStart)
	cgoThreadStart = unsafe.Pointer(C.cmemprof_cgo_thread_start)
}
