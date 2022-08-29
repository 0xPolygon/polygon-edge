// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

//go:build cgo && experimental_cmemprof && (linux || darwin)
// +build cgo
// +build experimental_cmemprof
// +build linux darwin

package profiler

import _ "gopkg.in/DataDog/dd-trace-go.v1/profiler/internal/cmemprof"
