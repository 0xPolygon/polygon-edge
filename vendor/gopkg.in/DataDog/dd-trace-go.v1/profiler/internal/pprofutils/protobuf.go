// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021 Datadog, Inc.

package pprofutils

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/google/pprof/profile"
)

// Protobuf converts from pprof's protobuf to folded text format.
type Protobuf struct {
	// SampleTypes causes the text output to begin with a header line listing
	// the sample types found in the profile. This is a custom extension to the
	// folded text format.
	SampleTypes bool
}

// Convert marshals the given protobuf profile into folded text format.
func (p Protobuf) Convert(protobuf *profile.Profile, text io.Writer) error {
	w := bufio.NewWriter(text)
	if p.SampleTypes {
		var sampleTypes []string
		for _, sampleType := range protobuf.SampleType {
			sampleTypes = append(sampleTypes, sampleType.Type+"/"+sampleType.Unit)
		}
		w.WriteString(strings.Join(sampleTypes, " ") + "\n")
	}
	if err := protobuf.Aggregate(true, true, false, false, false); err != nil {
		return err
	}
	protobuf = protobuf.Compact()
	sort.Slice(protobuf.Sample, func(i, j int) bool {
		return protobuf.Sample[i].Value[0] > protobuf.Sample[j].Value[0]
	})
	for _, sample := range protobuf.Sample {
		var frames []string
		for i := range sample.Location {
			loc := sample.Location[len(sample.Location)-i-1]
			for j := range loc.Line {
				line := loc.Line[len(loc.Line)-j-1]
				frames = append(frames, line.Function.Name)
			}
		}
		var values []string
		for _, val := range sample.Value {
			values = append(values, fmt.Sprintf("%d", val))
			if !p.SampleTypes {
				break
			}
		}
		fmt.Fprintf(
			w,
			"%s %s\n",
			strings.Join(frames, ";"),
			strings.Join(values, " "),
		)
	}
	return w.Flush()
}
