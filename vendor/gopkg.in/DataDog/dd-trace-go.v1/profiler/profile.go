// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package profiler

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/profiler/internal/extensions"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler/internal/pprofutils"

	"github.com/DataDog/gostackparse"
	pprofile "github.com/google/pprof/profile"
)

// ProfileType represents a type of profile that the profiler is able to run.
type ProfileType int

const (
	// HeapProfile reports memory allocation samples; used to monitor current
	// and historical memory usage, and to check for memory leaks.
	HeapProfile ProfileType = iota
	// CPUProfile determines where a program spends its time while actively consuming
	// CPU cycles (as opposed to while sleeping or waiting for I/O).
	CPUProfile
	// BlockProfile shows where goroutines block waiting on mutex and channel
	// operations. The block profile is not enabled by default and may cause
	// noticeable CPU overhead. We recommend against enabling it, see
	// DefaultBlockRate for more information.
	BlockProfile
	// MutexProfile reports the lock contentions. When you think your CPU is not fully utilized due
	// to a mutex contention, use this profile. Mutex profile is not enabled by default.
	MutexProfile
	// GoroutineProfile reports stack traces of all current goroutines
	GoroutineProfile
	// expGoroutineWaitProfile reports stack traces and wait durations for
	// goroutines that have been waiting or blocked by a syscall for > 1 minute
	// since the last GC. This feature is currently experimental and only
	// available within DD by setting the DD_PROFILING_WAIT_PROFILE env variable.
	expGoroutineWaitProfile
	// MetricsProfile reports top-line metrics associated with user-specified profiles
	MetricsProfile
)

// profileType holds the implementation details of a ProfileType.
type profileType struct {
	// Type gets populated automatically by ProfileType.lookup().
	Type ProfileType
	// Name specifies the profile name as used with pprof.Lookup(name) (in
	// collectGenericProfile) and returned by ProfileType.String(). For profile
	// types that don't use this approach (e.g. CPU) the name isn't used for
	// anything.
	Name string
	// Filename is the filename used for uploading the profile to the datadog
	// backend which is aware of them. Delta profiles are prefixed with "delta-"
	// automatically. In theory this could be derrived from the Name field, but
	// this isn't done due to idiosyncratic filename used by the
	// GoroutineProfile.
	Filename string
	// SupportsDelta indicates whether delta profiles can be computed for
	// this profile type, which is used to determine the final filename
	SupportsDelta bool
	// Collect collects the given profile and returns the data for it. Most
	// profiles will be in pprof format, i.e. gzip compressed proto buf data.
	Collect func(p *profiler) ([]byte, error)
}

// profileTypes maps every ProfileType to its implementation.
var profileTypes = map[ProfileType]profileType{
	CPUProfile: {
		Name:     "cpu",
		Filename: "cpu.pprof",
		Collect: func(p *profiler) ([]byte, error) {
			var buf bytes.Buffer
			if p.cfg.cpuProfileRate != 0 {
				// The profile has to be set each time before
				// profiling is started. Otherwise,
				// runtime/pprof.StartCPUProfile will set the
				// rate itself.
				runtime.SetCPUProfileRate(p.cfg.cpuProfileRate)
			}
			if err := p.startCPUProfile(&buf); err != nil {
				return nil, err
			}
			p.interruptibleSleep(p.cfg.cpuDuration)
			p.stopCPUProfile()
			return buf.Bytes(), nil
		},
	},
	// HeapProfile is complex due to how the Go runtime exposes it. It contains 4
	// sample types alloc_objects/count, alloc_space/bytes, inuse_objects/count,
	// inuse_space/bytes. The first two represent allocations over the lifetime
	// of the process, so we do delta profiling for them. The last two are
	// snapshots of the current heap state, so we leave them as-is.
	HeapProfile: {
		Name:     "heap",
		Filename: "heap.pprof",
		Collect: collectGenericProfile("heap", &pprofutils.Delta{SampleTypes: []pprofutils.ValueType{
			{Type: "alloc_objects", Unit: "count"},
			{Type: "alloc_space", Unit: "bytes"},
		}}),
		SupportsDelta: true,
	},
	MutexProfile: {
		Name:          "mutex",
		Filename:      "mutex.pprof",
		Collect:       collectGenericProfile("mutex", &pprofutils.Delta{}),
		SupportsDelta: true,
	},
	BlockProfile: {
		Name:          "block",
		Filename:      "block.pprof",
		Collect:       collectGenericProfile("block", &pprofutils.Delta{}),
		SupportsDelta: true,
	},
	GoroutineProfile: {
		Name:     "goroutine",
		Filename: "goroutines.pprof",
		Collect:  collectGenericProfile("goroutine", nil),
	},
	expGoroutineWaitProfile: {
		Name:     "goroutinewait",
		Filename: "goroutineswait.pprof",
		Collect: func(p *profiler) ([]byte, error) {
			if n := runtime.NumGoroutine(); n > p.cfg.maxGoroutinesWait {
				return nil, fmt.Errorf("skipping goroutines wait profile: %d goroutines exceeds DD_PROFILING_WAIT_PROFILE_MAX_GOROUTINES limit of %d", n, p.cfg.maxGoroutinesWait)
			}

			p.interruptibleSleep(p.cfg.period)

			var (
				now   = now()
				text  = &bytes.Buffer{}
				pprof = &bytes.Buffer{}
			)
			if err := p.lookupProfile("goroutine", text, 2); err != nil {
				return nil, err
			}
			err := goroutineDebug2ToPprof(text, pprof, now)
			return pprof.Bytes(), err
		},
	},
	MetricsProfile: {
		Name:     "metrics",
		Filename: "metrics.json",
		Collect: func(p *profiler) ([]byte, error) {
			var buf bytes.Buffer
			p.interruptibleSleep(p.cfg.period)
			err := p.met.report(now(), &buf)
			return buf.Bytes(), err
		},
	},
}

func collectGenericProfile(name string, delta *pprofutils.Delta) func(p *profiler) ([]byte, error) {
	return func(p *profiler) ([]byte, error) {
		var extra []*pprofile.Profile
		// TODO: add type safety for name == "heap" check and remove redunancy with profileType.Name.
		cAlloc, ok := extensions.GetCAllocationProfiler()
		switch {
		case ok && p.cfg.cmemprofEnabled && p.cfg.deltaProfiles && name == "heap":
			// For the heap profile, we'd also like to include C
			// allocations if that extension is enabled and have the
			// allocations show up in the same profile. Collect them
			// first before getting the regular heap snapshot so
			// that all allocations cover the same time period
			//
			// TODO: Support non-delta profiles for C allocations?
			cAlloc.Start(p.cfg.cmemprofRate)
			p.interruptibleSleep(p.cfg.period)
			profile, err := cAlloc.Stop()
			if err == nil {
				extra = append(extra, profile)
			}
		default:
			// In all cases, sleep until the end of the profile
			// period so that all profiles cover the same period of
			// time
			p.interruptibleSleep(p.cfg.period)
		}

		var buf bytes.Buffer
		err := p.lookupProfile(name, &buf, 0)
		data := buf.Bytes()
		if delta == nil || !p.cfg.deltaProfiles {
			return data, err
		}

		start := time.Now()
		delta, err := p.deltaProfile(name, delta, data, extra...)
		tags := append(p.cfg.tags.Slice(), fmt.Sprintf("profile_type:%s", name))
		p.cfg.statsd.Timing("datadog.profiling.go.delta_time", time.Since(start), tags, 1)
		if err != nil {
			return nil, fmt.Errorf("delta profile error: %s", err)
		}
		return delta.data, err
	}
}

// lookup returns t's profileType implementation.
func (t ProfileType) lookup() profileType {
	c, ok := profileTypes[t]
	if ok {
		c.Type = t
		return c
	}
	return profileType{
		Type:     t,
		Name:     "unknown",
		Filename: "unknown",
		Collect: func(_ *profiler) ([]byte, error) {
			return nil, errors.New("profile type not implemented")
		},
	}
}

// String returns the name of the profile.
func (t ProfileType) String() string {
	return t.lookup().Name
}

// Filename is the identifier used on upload.
func (t ProfileType) Filename() string {
	return t.lookup().Filename
}

// Tag used on profile metadata
func (t ProfileType) Tag() string {
	return fmt.Sprintf("profile_type:%s", t)
}

// profile specifies a profiles data (gzipped protobuf, json), and the types contained within it.
type profile struct {
	// name indicates profile type and format (e.g. cpu.pprof, metrics.json)
	name string
	data []byte
}

// batch is a collection of profiles of different types, collected at roughly the same time. It maps
// to what the Datadog UI calls a profile.
type batch struct {
	seq        uint64 // seq is the value of the profile_seq tag
	start, end time.Time
	host       string
	profiles   []*profile
}

func (b *batch) addProfile(p *profile) {
	b.profiles = append(b.profiles, p)
}

func (p *profiler) runProfile(pt ProfileType) ([]*profile, error) {
	start := now()
	t := pt.lookup()
	data, err := t.Collect(p)
	if err != nil {
		return nil, err
	}
	end := now()
	tags := append(p.cfg.tags.Slice(), pt.Tag())
	filename := t.Filename
	// TODO(fg): Consider making Collect() return the filename.
	if p.cfg.deltaProfiles && t.SupportsDelta {
		filename = "delta-" + filename
	}
	p.cfg.statsd.Timing("datadog.profiling.go.collect_time", end.Sub(start), tags, 1)
	return []*profile{{name: filename, data: data}}, nil
}

// deltaProfile derives the delta profile between curData and the previous
// profile. If extra profiles are provided, they will be merged into the final
// profile after computing the delta profile.
func (p *profiler) deltaProfile(name string, delta *pprofutils.Delta, curData []byte, extra ...*pprofile.Profile) (*profile, error) {
	curProf, err := pprofile.ParseData(curData)
	if err != nil {
		return nil, fmt.Errorf("delta prof parse: %v", err)
	}
	var deltaData []byte
	p.mu.Lock()
	prevProf := p.prev[name]
	p.mu.Unlock()
	if prevProf == nil {
		// First time deltaProfile gets called for a type, there is no prevProf. In
		// this case we emit the current profile as a delta profile.
		deltaData = curData
	} else {
		// Delta profiling is also implemented in the Go core, see commit below.
		// Unfortunately the core implementation isn't resuable via a API, so we do
		// our own delta calculation below.
		// https://github.com/golang/go/commit/2ff1e3ebf5de77325c0e96a6c2a229656fc7be50#diff-94594f8f13448da956b02997e50ca5a156b65085993e23bbfdda222da6508258R303-R304
		deltaProf, err := delta.Convert(prevProf, curProf, extra...)
		if err != nil {
			return nil, fmt.Errorf("delta prof merge: %v", err)
		}
		// TimeNanos is supposed to be the time the profile was collected, see
		// https://github.com/google/pprof/blob/master/proto/profile.proto.
		deltaProf.TimeNanos = curProf.TimeNanos
		// DurationNanos is the time period covered by the profile.
		deltaProf.DurationNanos = curProf.TimeNanos - prevProf.TimeNanos
		deltaBuf := &bytes.Buffer{}
		if err := deltaProf.Write(deltaBuf); err != nil {
			return nil, fmt.Errorf("delta prof write: %v", err)
		}
		deltaData = deltaBuf.Bytes()
	}
	// Keep the most recent profiles in memory for future diffing. This needs to
	// be taken into account when enforcing memory limits going forward.
	p.mu.Lock()
	p.prev[name] = curProf
	p.mu.Unlock()
	return &profile{data: deltaData}, nil
}

func goroutineDebug2ToPprof(r io.Reader, w io.Writer, t time.Time) (err error) {
	// gostackparse.Parse() has been extensively tested and should not crash
	// under any circumstances, but we really want to avoid crashing a customers
	// applications, so this code will recover from any unexpected panics and
	// return them as an error instead.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	goroutines, errs := gostackparse.Parse(r)

	functionID := uint64(1)
	locationID := uint64(1)

	p := &pprofile.Profile{
		TimeNanos: t.UnixNano(),
	}
	m := &pprofile.Mapping{ID: 1, HasFunctions: true}
	p.Mapping = []*pprofile.Mapping{m}
	p.SampleType = []*pprofile.ValueType{
		{
			Type: "waitduration",
			Unit: "nanoseconds",
		},
	}

	for _, g := range goroutines {
		sample := &pprofile.Sample{
			Value: []int64{g.Wait.Nanoseconds()},
			Label: map[string][]string{
				"state":   {g.State}, // TODO(fg) split into atomicstatus/waitreason?
				"lockedm": {fmt.Sprintf("%t", g.LockedToThread)},
			},
			NumUnit:  map[string][]string{"goid": {"id"}},
			NumLabel: map[string][]int64{"goid": {int64(g.ID)}},
		}

		// Treat the frame that created this goroutine as part of the stack so it
		// shows up in the stack trace / flame graph. Hopefully this will be more
		// useful than confusing for people.
		if g.CreatedBy != nil {
			// TODO(fg) should we modify the function name to include "created by"?
			g.Stack = append(g.Stack, g.CreatedBy)
		}

		// Based on internal discussion, the current strategy is to use virtual
		// frames to indicate truncated stacks, see [1] for how python/jd does it.
		// [1] https://github.com/DataDog/dd-trace-py/blob/e933d2485b9019a7afad7127f7c0eb541341cdb7/ddtrace/profiling/exporter/pprof.pyx#L117-L121
		if g.FramesElided {
			g.Stack = append(g.Stack, &gostackparse.Frame{
				Func: "...additional frames elided...",
			})
		}

		for _, call := range g.Stack {
			function := &pprofile.Function{
				ID:       functionID,
				Name:     call.Func,
				Filename: call.File,
			}
			p.Function = append(p.Function, function)
			functionID++

			location := &pprofile.Location{
				ID:      locationID,
				Mapping: m,
				Line: []pprofile.Line{{
					Function: function,
					Line:     int64(call.Line),
				}},
			}
			p.Location = append(p.Location, location)
			locationID++

			sample.Location = append(sample.Location, location)
		}

		p.Sample = append(p.Sample, sample)
	}

	// Put the error message in the pprof profiles as comments in case we need to
	// debug issues at some point.
	// TODO(fg) would be nice to also have a metric counter for this
	for _, err := range errs {
		p.Comments = append(p.Comments, "error: "+err.Error())
	}

	if err := p.CheckValid(); err != nil {
		return fmt.Errorf("marshalGoroutineDebug2Profile: %s", err)
	} else if err := p.Write(w); err != nil {
		return fmt.Errorf("marshalGoroutineDebug2Profile: %s", err)
	}
	return nil
}

// now returns current time in UTC.
func now() time.Time {
	return time.Now().UTC()
}
