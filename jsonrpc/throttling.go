package jsonrpc

import (
	"container/heap"
	"errors"
	"sync"
	"time"
)

var errRequestLimitExceeded = errors.New("request limit exceeded")

type Throttling struct {
	requestsPerSeconds int
	requests           timeQueue
	lock               sync.Mutex
}

func NewThrottling(requestsPerSeconds int) *Throttling {
	return &Throttling{
		requestsPerSeconds: requestsPerSeconds,
	}
}

func (t *Throttling) AttemptRequest() error {
	t.lock.Lock()
	defer t.lock.Unlock()

	currTime := time.Now().UTC()

	// remove all old requests
	for t.requests.Len() > 0 && currTime.Sub(t.requests[0]) > time.Second {
		heap.Pop(&t.requests)
	}

	// if too many requests in one second return error
	if t.requests.Len() == t.requestsPerSeconds {
		return errRequestLimitExceeded
	}

	heap.Push(&t.requests, currTime)

	return nil
}

type timeQueue []time.Time

func (t *timeQueue) Len() int {
	return len(*t)
}

func (t *timeQueue) Swap(i, j int) {
	(*t)[i], (*t)[j] = (*t)[j], (*t)[i]
}

func (t *timeQueue) Push(x interface{}) {
	if time, ok := x.(time.Time); ok {
		*t = append(*t, time)
	}
}

func (t *timeQueue) Pop() interface{} {
	x := (*t)[len(*t)-1]
	*t = (*t)[0 : len(*t)-1]

	return x
}

func (t *timeQueue) Less(i, j int) bool {
	return (*t)[i].Compare((*t)[j]) < 0
}
