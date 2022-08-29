// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

//go:build cgo
// +build cgo

// Package cmemprof profiles C memory allocations (malloc, calloc, realloc, etc.)
//
// Importing this package in a program will replace malloc, calloc, and realloc
// with wrappers which will sample allocations and record them to a profile.
//
// To use this package:
//
//	f, _ := os.Create("cmem.pprof")
//	var profiler cmemprof.Profile
//	profiler.Start(500)
//	// ... do allocations
//	profile, err := profiler.Stop()
//
// Building this package on Linux requires a non-standard linker flag to wrap
// the alloaction functions. For Go versions < 1.15, cgo won't allow the flag so
// it has to be explicitly allowed by setting the following environment variable
// when building a program that uses this package:
//
//	export CGO_LDFLAGS_ALLOW="-Wl,--wrap=.*"
package cmemprof

/*
#cgo CFLAGS: -g -O2 -fno-omit-frame-pointer
#cgo linux LDFLAGS: -pthread -ldl
#cgo linux LDFLAGS: -Wl,--wrap=calloc
#cgo linux LDFLAGS: -Wl,--wrap=malloc
#cgo linux LDFLAGS: -Wl,--wrap=realloc
#cgo linux LDFLAGS: -Wl,--wrap=valloc
#cgo linux LDFLAGS: -Wl,--wrap=aligned_alloc
#cgo linux LDFLAGS: -Wl,--wrap=posix_memalign
#cgo darwin LDFLAGS: -ldl -pthread
#include <stdint.h> // for uintptr_t

#include "profiler.h"
*/
import "C"

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/pprof/profile"

	"gopkg.in/DataDog/dd-trace-go.v1/profiler/internal/extensions"
)

func init() {
	extensions.SetCAllocationProfiler(new(Profile))
}

// callStack is a sequence of program counters representing a call to malloc,
// calloc, etc. callStack is 0-terminated.
//
// TODO: make callStack larger, or support variable-length call stacks. See
// https://cs.opensource.google/go/go/+/master:src/runtime/pprof/map.go for an
// example of a hash map keyed by variable-length call stacks
type callStack [32]uintptr

// Stack returns the call stack without any trailing 0 program counters
func (c *callStack) Stack() []uintptr {
	for i, pc := range c {
		if pc == 0 {
			return c[:i]
		}
	}
	return c[:]
}

type aggregatedSample struct {
	// bytes is the total number of bytes allocated
	bytes uint
	// count is the number of times this event has been observed
	count int
}

// Profile provides access to a C memory allocation profiler based on
// instrumenting malloc, calloc, and realloc.
type Profile struct {
	mu      sync.Mutex
	active  bool
	samples map[callStack]*aggregatedSample

	// SamplingRate is the value, in bytes, such that an average of one
	// sample will be recorded for every SamplingRate bytes allocated.  An
	// allocation of N bytes will be recorded with probability min(1, N /
	// SamplingRate).
	SamplingRate int
}

// activeProfile is an atomic value since recordAllocationSample and Profile.Start could be called concurrently.
var activeProfile atomic.Value

// Start begins profiling C memory allocations.
func (c *Profile) Start(rate int) {
	if rate <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active {
		return
	}
	c.active = true
	// We blow away the samples from the previous round of profiling so that
	// the final profile returned by Stop only has the allocations between
	// Stop and the preceding call to Start
	//
	// Creating a new map rather than setting each sample count/size to 0 or
	// deleting every entry does mean we need to do some duplicate work for
	// each round of profiling. However, starting with a new map each time
	// avoids the behavior of the runtime heap, block, and mutex profiles
	// which never remove samples for the duration of the profile. In
	// adittion, Go maps never shrink as of Go 1.18, so even if some space
	// is reused after clearing a map, the total amount of memory used by
	// the map only ever increases.
	c.samples = make(map[callStack]*aggregatedSample)
	c.SamplingRate = rate
	activeProfile.Store(c)
	C.cgo_heap_profiler_set_sampling_rate(C.size_t(rate))
}

func (c *Profile) insert(pcs callStack, size uint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sample := c.samples[pcs]
	if sample == nil {
		sample = new(aggregatedSample)
		c.samples[pcs] = sample
	}
	rate := uint(c.SamplingRate)
	if size >= rate {
		sample.bytes += size
		sample.count++
	} else {
		// The allocation was sample with probability p = size / rate.
		// So we assume there were actually (1 / p) similar allocations
		// for a total size of (1 / p) * size = rate
		sample.bytes += rate
		sample.count += int(float64(rate) / float64(size))
	}
}

// Stop cancels memory profiling and waits for the profile to be written to the
// io.Writer passed to Start. Returns any error from writing the profile.
func (c *Profile) Stop() (*profile.Profile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	C.cgo_heap_profiler_set_sampling_rate(0)
	if !c.active {
		return nil, fmt.Errorf("profiling isn't started")
	}
	c.active = false
	p := c.build()
	if err := p.CheckValid(); err != nil {
		return nil, fmt.Errorf("bad profile: %s", err)
	}
	return p, nil
}
