// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package profiler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
)

// maxRetries specifies the maximum number of retries to have when an error occurs.
const maxRetries = 2

var errOldAgent = errors.New("Datadog Agent is not accepting profiles. Agent-based profiling deployments " +
	"require Datadog Agent >= 7.20")

// upload tries to upload a batch of profiles. It has retry and backoff mechanisms.
func (p *profiler) upload(bat batch) error {
	statsd := p.cfg.statsd
	var err error
	for i := 0; i < maxRetries; i++ {
		select {
		case <-p.exit:
			return nil
		default:
		}

		err = p.doRequest(bat)
		if rerr, ok := err.(*retriableError); ok {
			statsd.Count("datadog.profiling.go.upload_retry", 1, nil, 1)
			wait := time.Duration(rand.Int63n(p.cfg.period.Nanoseconds())) * time.Nanosecond
			log.Error("Uploading profile failed: %v. Trying again in %s...", rerr, wait)
			p.interruptibleSleep(wait)
			continue
		}
		if err != nil {
			statsd.Count("datadog.profiling.go.upload_error", 1, nil, 1)
		} else {
			statsd.Count("datadog.profiling.go.upload_success", 1, nil, 1)
			var b int64
			for _, p := range bat.profiles {
				b += int64(len(p.data))
			}
			statsd.Count("datadog.profiling.go.uploaded_profile_bytes", b, nil, 1)
		}
		return err
	}
	return fmt.Errorf("failed after %d retries, last error was: %v", maxRetries, err)
}

// retriableError is an error returned by the server which may be retried at a later time.
type retriableError struct{ err error }

// Error implements error.
func (e retriableError) Error() string { return e.err.Error() }

// doRequest makes an HTTP POST request to the Datadog Profiling API with the
// given profile.
func (p *profiler) doRequest(bat batch) error {
	tags := append(p.cfg.tags.Slice(),
		fmt.Sprintf("service:%s", p.cfg.service),
		fmt.Sprintf("env:%s", p.cfg.env),
		// The profile_seq tag can be used to identify the first profile
		// uploaded by a given runtime-id, identify missing profiles, etc.. See
		// PROF-5612 (internal) for more details.
		fmt.Sprintf("profile_seq:%d", bat.seq),
	)
	contentType, body, err := encode(bat, tags)
	if err != nil {
		return err
	}
	funcExit := make(chan struct{})
	defer close(funcExit)
	// uploadTimeout is guaranteed to be >= 0, see newProfiler.
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.uploadTimeout)
	go func() {
		select {
		case <-p.exit:
		case <-funcExit:
		}
		cancel()
	}()
	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.targetURL, body)
	if err != nil {
		return err
	}
	if p.cfg.apiKey != "" {
		req.Header.Set("DD-API-KEY", p.cfg.apiKey)
	}
	if containerID != "" {
		req.Header.Set("Datadog-Container-ID", containerID)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := p.cfg.httpClient.Do(req)
	if err != nil {
		return &retriableError{err}
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 5 {
		// 5xx can be retried
		return &retriableError{errors.New(resp.Status)}
	}
	if resp.StatusCode == 404 && p.cfg.targetURL == p.cfg.agentURL {
		// 404 from the agent means we have an old agent version without profiling endpoint
		return errOldAgent
	}
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		// Success!
		return nil
	}
	return errors.New(resp.Status)
}

// encode encodes the profile as a multipart mime request.
func encode(bat batch, tags []string) (contentType string, body io.Reader, err error) {
	var buf bytes.Buffer

	mw := multipart.NewWriter(&buf)
	// write all of the profile metadata (including some useless ones)
	// with a small helper function that makes error tracking less verbose.
	writeField := func(k, v string) {
		if err == nil {
			err = mw.WriteField(k, v)
		}
	}
	writeField("version", "3")
	writeField("family", "go")
	writeField("start", bat.start.Format(time.RFC3339))
	writeField("end", bat.end.Format(time.RFC3339))
	if bat.host != "" {
		writeField("tags[]", fmt.Sprintf("host:%s", bat.host))
	}
	writeField("tags[]", "runtime:go")
	for _, tag := range tags {
		writeField("tags[]", tag)
	}
	if err != nil {
		return "", nil, err
	}
	for _, p := range bat.profiles {
		formFile, err := mw.CreateFormFile(fmt.Sprintf("data[%s]", p.name), "pprof-data")
		if err != nil {
			return "", nil, err
		}
		if _, err := formFile.Write(p.data); err != nil {
			return "", nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return "", nil, err
	}
	return mw.FormDataContentType(), &buf, nil
}
