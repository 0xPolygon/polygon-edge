// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.
//
// The parseMapping code comes from the Google Go source code. The code is
// licensed under the BSD 3-clause license (a copy is in LICENSE-go in this
// directory, see also LICENSE-3rdparty.csv) and comes with the following
// notice:
//
// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmemprof

import (
	"bytes"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/google/pprof/profile"
)

var defaultMapping = &profile.Mapping{
	ID:   1,
	File: os.Args[0],
}

var mappings []*profile.Mapping

func getMapping(addr uint64) *profile.Mapping {
	i := sort.Search(len(mappings), func(n int) bool {
		return mappings[n].Limit >= addr
	})
	if i == len(mappings) || addr < mappings[i].Start {
		return defaultMapping
	}
	return mappings[i]
}

func init() {
	if runtime.GOOS != "linux" {
		mappings = append(mappings, defaultMapping)
		return
	}

	// To have a more accurate profile, we need to provide mappings for the
	// executable and linked libraries, since the profile might contain
	// samples from outside the executable if the allocation profiler is
	// used in conjunction with a cgo traceback library.
	//
	// We can find this information in /proc/self/maps on Linux. Code comes
	// from mappings with executable permissions (r-xp), and a record looks
	// like
	//	address           perms offset  dev   inode       pathname
	//	00400000-00452000 r-xp 00000000 08:02 173521      /usr/bin/dbus-daemon
	// (see "man 5 proc" for details)

	data, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		mappings = append(mappings, defaultMapping)
		return
	}

	mappings = parseMappings(data)
}

// bytes.Cut, but backported so we can still support Go 1.17
func bytesCut(s, sep []byte) (before, after []byte, found bool) {
	if i := bytes.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, nil, false
}

func stringsCut(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

func parseMappings(data []byte) []*profile.Mapping {
	// This code comes from parseProcSelfMaps in the
	// official Go repository. See
	//	https://go.googlesource.com/go/+/refs/tags/go1.18.4/src/runtime/pprof/proto.go#596

	var results []*profile.Mapping
	var line []byte
	// next removes and returns the next field in the line.
	// It also removes from line any spaces following the field.
	next := func() []byte {
		var f []byte
		f, line, _ = bytesCut(line, []byte(" "))
		line = bytes.TrimLeft(line, " ")
		return f
	}

	for len(data) > 0 {
		line, data, _ = bytesCut(data, []byte("\n"))
		addr := next()
		loStr, hiStr, ok := stringsCut(string(addr), "-")
		if !ok {
			continue
		}
		lo, err := strconv.ParseUint(loStr, 16, 64)
		if err != nil {
			continue
		}
		hi, err := strconv.ParseUint(hiStr, 16, 64)
		if err != nil {
			continue
		}
		perm := next()
		if len(perm) < 4 || perm[2] != 'x' {
			// Only interested in executable mappings.
			continue
		}
		offset, err := strconv.ParseUint(string(next()), 16, 64)
		if err != nil {
			continue
		}
		next()          // dev
		inode := next() // inode
		if line == nil {
			continue
		}
		file := string(line)

		// Trim deleted file marker.
		deletedStr := " (deleted)"
		deletedLen := len(deletedStr)
		if len(file) >= deletedLen && file[len(file)-deletedLen:] == deletedStr {
			file = file[:len(file)-deletedLen]
		}

		if len(inode) == 1 && inode[0] == '0' && file == "" {
			// Huge-page text mappings list the initial fragment of
			// mapped but unpopulated memory as being inode 0.
			// Don't report that part.
			// But [vdso] and [vsyscall] are inode 0, so let non-empty file names through.
			continue
		}

		results = append(results, &profile.Mapping{
			ID:     uint64(len(results) + 1),
			Start:  lo,
			Limit:  hi,
			Offset: offset,
			File:   file,
			// Go normally sets the HasFunctions, HasLineNumbers,
			// etc. fields for the main executable when it consists
			// solely of Go code. However, users of this C
			// allocation profiler will necessarily be using non-Go
			// code and we don't know whether there are functions,
			// line numbers, etc. available for the non-Go code.
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Start < results[j].Start
	})

	return results
}

func (c *Profile) build() *profile.Profile {
	// TODO: can we be sure that there won't be other allocation samples
	// ongoing that write to the sample map? Right now it's called with c.mu
	// held but it wouldn't be good if this (probably expensive) function
	// was holding the same lock that recordAllocationSample also wants to
	// hold

	// This profile is intended to be merged into the Go runtime allocation
	// profile.  The profile.Merge function requires that several fields in
	// the merged profile.Profile objects match in order for the merge to be
	// successful. Refer to
	//
	// https://pkg.go.dev/github.com/google/pprof/profile#Merge
	//
	// In particular, the PeriodType and SampleType fields must match, and
	// every sample must have the correct number of values. The TimeNanos
	// field can be left 0, and the TimeNanos field of the Go allocation
	// profile will be used.
	p := &profile.Profile{}
	p.PeriodType = &profile.ValueType{Type: "space", Unit: "bytes"}
	p.Period = 1
	p.Mapping = mappings
	p.SampleType = []*profile.ValueType{
		{
			Type: "alloc_objects",
			Unit: "count",
		},
		{
			Type: "alloc_space",
			Unit: "bytes",
		},
		// This profiler doesn't actually do heap profiling yet, but in
		// order to view Go allocation profiles and C allocation
		// profiles at the same time, the sample types need to be the
		// same
		{
			Type: "inuse_objects",
			Unit: "count",
		},
		{
			Type: "inuse_space",
			Unit: "bytes",
		},
	}
	// TODO: move these cache up into Profile?
	functions := make(map[string]*profile.Function)
	locations := make(map[uint64]*profile.Location)
	for stack, event := range c.samples {
		psample := &profile.Sample{
			Value: []int64{int64(event.count), int64(event.bytes), 0, 0},
		}
		frames := runtime.CallersFrames(stack.Stack())
		for {
			frame, ok := frames.Next()
			if !ok {
				break
			}
			// runtime.Callers has a skip argument but we can't skip
			// the exported allocation sample function with it, so
			// we manually prune it here.
			if frame.Function == "recordAllocationSample" {
				continue
			}
			if strings.HasPrefix(frame.Function, "profile_allocation") {
				continue
			}
			addr := uint64(frame.PC)
			loc, ok := locations[addr]
			if !ok {
				loc = &profile.Location{
					ID:      uint64(len(locations)) + 1,
					Mapping: getMapping(addr),
					Address: addr,
				}
				function, ok := functions[frame.Function]
				if !ok {
					function = &profile.Function{
						ID:       uint64(len(p.Function)) + 1,
						Filename: frame.File,
						Name:     frame.Function,
					}
					// On Linux, allocation functions end up
					// with a "__wrap_" prefix, which we
					// remove to avoid confusion ("where did
					// __wrap_malloc come from?")
					function.Name = strings.TrimPrefix(function.Name, "__wrap_")
					p.Function = append(p.Function, function)
				}
				loc.Line = append(loc.Line, profile.Line{
					Function: function,
					Line:     int64(frame.Line),
				})
				locations[addr] = loc
				p.Location = append(p.Location, loc)
			}
			psample.Location = append(psample.Location, loc)
		}
		p.Sample = append(p.Sample, psample)
	}
	return p
}
