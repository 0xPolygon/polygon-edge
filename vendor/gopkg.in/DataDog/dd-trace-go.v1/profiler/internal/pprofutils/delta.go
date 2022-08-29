// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021 Datadog, Inc.

package pprofutils

import (
	"errors"

	"github.com/google/pprof/profile"
)

// Delta describes how to compute the delta between two profiles and implements
// the conversion.
type Delta struct {
	// SampleTypes limits the delta calcultion to the given sample types. Other
	// sample types will retain the values of profile b. The defined sample types
	// must exist in the profile, otherwise derivation will fail with an error.
	// If the slice is empty, all sample types are subject to delta profile
	// derivation.
	//
	// The use case for this for this is to deal with the heap profile which
	// contains alloc and inuse sample types, but delta profiling makes no sense
	// for the latter.
	SampleTypes []ValueType
}

// Convert computes the delta between all values b-a and returns them as a new
// profile. Samples that end up with a delta of 0 are dropped. WARNING: Profile
// a will be mutated by this function. You should pass a copy if that's
// undesirable.
//
// Other profiles that should be merged into the resulting profile can be passed
// through the extra parameter.
func (d Delta) Convert(a, b *profile.Profile, extra ...*profile.Profile) (*profile.Profile, error) {
	ratios := make([]float64, len(a.SampleType))

	found := 0
	for i, st := range a.SampleType {
		// Empty c.SampleTypes means we calculate the delta for every st
		if len(d.SampleTypes) == 0 {
			ratios[i] = -1
			continue
		}

		// Otherwise we only calcuate the delta for any st that is listed in
		// c.SampleTypes. st's not listed in there will default to ratio 0, which
		// means we delete them from pa, so only the pb values remain in the final
		// profile.
		for _, deltaSt := range d.SampleTypes {
			if deltaSt.Type == st.Type && deltaSt.Unit == st.Unit {
				ratios[i] = -1
				found++
			}
		}
	}
	if found != len(d.SampleTypes) {
		return nil, errors.New("one or more sample type(s) was not found in the profile")
	}

	a.ScaleN(ratios)

	profiles := make([]*profile.Profile, 0, 2+len(extra))
	profiles = append(profiles, a, b)
	profiles = append(profiles, extra...)
	delta, err := profile.Merge(profiles)
	if err != nil {
		return nil, err
	}
	return delta, delta.CheckValid()
}
