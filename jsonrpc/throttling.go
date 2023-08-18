package jsonrpc

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sync/semaphore"
)

var errRequestLimitExceeded = errors.New("request limit exceeded")

// Throttling provides functionality which limits number of concurrent requests
type Throttling struct {
	sem     *semaphore.Weighted
	timeout time.Duration
}

// NewThrottling creates new throttling and limits number of concurrent requests to maximumConcurrentRequests
func NewThrottling(maximumConcurrentRequests uint64, timeout time.Duration) *Throttling {
	return &Throttling{
		sem:     semaphore.NewWeighted(int64(maximumConcurrentRequests)),
		timeout: timeout,
	}
}

// AttemptRequest returns an error if more than the maximum concurrent requests are currently being executed
func (t *Throttling) AttemptRequest(
	parentCtx context.Context,
	requestHandler func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := context.WithTimeout(parentCtx, t.timeout)
	defer cancel()

	if err := t.sem.Acquire(ctx, 1); err != nil {
		return nil, errRequestLimitExceeded
	}

	defer t.sem.Release(1)

	return requestHandler()
}
