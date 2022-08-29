// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

#ifndef PROFILER_H
#define PROFILER_H
#include <stddef.h>

// cgo_heap_profiler_set_sampling_rate configures profiling to capture 1/hz of
// allocations, and returns the previous rate. If hz <= 0, then sampling is
// disabled.
void cgo_heap_profiler_set_sampling_rate(size_t hz);

#endif
