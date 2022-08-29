// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021 Datadog, Inc.

package pprofutils

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/pprof/profile"
)

// Text converts from folded text to protobuf format.
type Text struct{}

// Convert parses the given text and returns it as protobuf profile.
func (c Text) Convert(text io.Reader) (*profile.Profile, error) {
	var (
		functionID = uint64(1)
		locationID = uint64(1)
		p          = &profile.Profile{
			TimeNanos: time.Now().UnixNano(),
			SampleType: []*profile.ValueType{{
				Type: "samples",
				Unit: "count",
			}},
			// Without his, Delta.Convert() fails in profile.Merge(). Perhaps an
			// issue that's worth reporting upstream.
			PeriodType: &profile.ValueType{},
		}
	)

	m := &profile.Mapping{ID: 1, HasFunctions: true}
	p.Mapping = []*profile.Mapping{m}

	lines, err := io.ReadAll(text)
	if err != nil {
		return nil, err
	}
	for n, line := range strings.Split(string(lines), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// custom extension: first line can contain header that looks like this:
		// "samples/count duration/nanoseconds" to describe the sample types
		if n == 0 && looksLikeHeader(line) {
			p.SampleType = nil
			for _, sampleType := range strings.Split(line, " ") {
				parts := strings.Split(sampleType, "/")
				if len(parts) != 2 {
					return nil, fmt.Errorf("bad header: %d: %q", n, line)
				}
				p.SampleType = append(p.SampleType, &profile.ValueType{
					Type: parts[0],
					Unit: parts[1],
				})
			}
			continue
		}

		parts := strings.Split(line, " ")
		if len(parts) != len(p.SampleType)+1 {
			return nil, fmt.Errorf("bad line: %d: %q", n, line)
		}

		stack := strings.Split(parts[0], ";")
		sample := &profile.Sample{}
		for _, valS := range parts[1:] {
			val, err := strconv.ParseInt(valS, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("bad line: %d: %q: %s", n, line, err)
			}
			sample.Value = append(sample.Value, val)
		}

		for i := range stack {
			frame := stack[len(stack)-i-1]
			function := &profile.Function{
				ID:   functionID,
				Name: frame,
			}
			p.Function = append(p.Function, function)
			functionID++

			location := &profile.Location{
				ID:      locationID,
				Mapping: m,
				Line:    []profile.Line{{Function: function}},
			}
			p.Location = append(p.Location, location)
			locationID++

			sample.Location = append(sample.Location, location)
		}

		p.Sample = append(p.Sample, sample)
	}
	return p, p.CheckValid()
}

// looksLikeHeader returns true if the line looks like this:
// "samples/count duration/nanoseconds". The heuristic used for detecting this
// is to check if every space separated value contains a "/" character.
func looksLikeHeader(line string) bool {
	for _, sampleType := range strings.Split(line, " ") {
		if !strings.Contains(sampleType, "/") {
			return false
		}
	}
	return true
}
