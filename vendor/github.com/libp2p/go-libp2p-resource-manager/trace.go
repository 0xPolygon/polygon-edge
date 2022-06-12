package rcmgr

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
)

type trace struct {
	path string

	ctx    context.Context
	cancel func()
	closed chan struct{}

	mx   sync.Mutex
	done bool
	pend []interface{}
}

func WithTrace(path string) Option {
	return func(r *resourceManager) error {
		r.trace = &trace{path: path}
		return nil
	}
}

const (
	traceStartEvt              = "start"
	traceCreateScopeEvt        = "create_scope"
	traceDestroyScopeEvt       = "destroy_scope"
	traceReserveMemoryEvt      = "reserve_memory"
	traceBlockReserveMemoryEvt = "block_reserve_memory"
	traceReleaseMemoryEvt      = "release_memory"
	traceAddStreamEvt          = "add_stream"
	traceBlockAddStreamEvt     = "block_add_stream"
	traceRemoveStreamEvt       = "remove_stream"
	traceAddConnEvt            = "add_conn"
	traceBlockAddConnEvt       = "block_add_conn"
	traceRemoveConnEvt         = "remove_conn"
)

type traceEvt struct {
	Type string

	Scope string `json:",omitempty"`

	Limit interface{} `json:",omitempty"`

	Priority uint8 `json:",omitempty"`

	Delta    int64 `json:",omitempty"`
	DeltaIn  int   `json:",omitempty"`
	DeltaOut int   `json:",omitempty"`

	Memory int64 `json:",omitempty"`

	StreamsIn  int `json:",omitempty"`
	StreamsOut int `json:",omitempty"`

	ConnsIn  int `json:",omitempty"`
	ConnsOut int `json:",omitempty"`

	FD int `json:",omitempty"`
}

func (t *trace) push(evt interface{}) {
	t.mx.Lock()
	defer t.mx.Unlock()

	if t.done {
		return
	}

	t.pend = append(t.pend, evt)
}

func (t *trace) background(out io.WriteCloser) {
	defer close(t.closed)
	defer out.Close()

	gzOut := gzip.NewWriter(out)
	defer gzOut.Close()

	jsonOut := json.NewEncoder(gzOut)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var pend []interface{}

	getEvents := func() {
		t.mx.Lock()
		tmp := t.pend
		t.pend = pend[:0]
		pend = tmp
		t.mx.Unlock()
	}

	for {
		select {
		case <-ticker.C:
			getEvents()

			if len(pend) == 0 {
				continue
			}

			if err := t.writeEvents(pend, jsonOut); err != nil {
				log.Warnf("error writing rcmgr trace: %s", err)
				t.mx.Lock()
				t.done = true
				t.mx.Unlock()
				return
			}

			if err := gzOut.Flush(); err != nil {
				log.Warnf("error flushing rcmgr trace: %s", err)
				t.mx.Lock()
				t.done = true
				t.mx.Unlock()
				return
			}

		case <-t.ctx.Done():
			getEvents()

			if len(pend) == 0 {
				return
			}

			if err := t.writeEvents(pend, jsonOut); err != nil {
				log.Warnf("error writing rcmgr trace: %s", err)
				return
			}

			if err := gzOut.Flush(); err != nil {
				log.Warnf("error flushing rcmgr trace: %s", err)
			}

			return
		}
	}
}

func (t *trace) writeEvents(pend []interface{}, jout *json.Encoder) error {
	for _, e := range pend {
		if err := jout.Encode(e); err != nil {
			return err
		}
	}

	return nil
}

func (t *trace) Start(limits Limiter) error {
	if t == nil {
		return nil
	}

	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.closed = make(chan struct{})

	out, err := os.OpenFile(t.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil
	}

	go t.background(out)

	t.push(traceEvt{
		Type:  traceStartEvt,
		Limit: limits,
	})

	return nil
}

func (t *trace) Close() error {
	if t == nil {
		return nil
	}

	t.mx.Lock()

	if t.done {
		t.mx.Unlock()
		return nil
	}

	t.cancel()
	t.done = true
	t.mx.Unlock()

	<-t.closed
	return nil
}

func (t *trace) CreateScope(scope string, limit Limit) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:  traceCreateScopeEvt,
		Scope: scope,
		Limit: limit,
	})
}

func (t *trace) DestroyScope(scope string) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:  traceDestroyScopeEvt,
		Scope: scope,
	})
}

func (t *trace) ReserveMemory(scope string, prio uint8, size, mem int64) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:     traceReserveMemoryEvt,
		Scope:    scope,
		Priority: prio,
		Delta:    size,
		Memory:   mem,
	})
}

func (t *trace) BlockReserveMemory(scope string, prio uint8, size, mem int64) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:     traceBlockReserveMemoryEvt,
		Scope:    scope,
		Priority: prio,
		Delta:    size,
		Memory:   mem,
	})
}

func (t *trace) ReleaseMemory(scope string, size, mem int64) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:   traceReleaseMemoryEvt,
		Scope:  scope,
		Delta:  size,
		Memory: mem,
	})
}

func (t *trace) AddStream(scope string, dir network.Direction, nstreamsIn, nstreamsOut int) {
	if t == nil {
		return
	}

	var deltaIn, deltaOut int
	if dir == network.DirInbound {
		deltaIn = 1
	} else {
		deltaOut = 1
	}

	t.push(traceEvt{
		Type:       traceAddStreamEvt,
		Scope:      scope,
		DeltaIn:    deltaIn,
		DeltaOut:   deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

func (t *trace) BlockAddStream(scope string, dir network.Direction, nstreamsIn, nstreamsOut int) {
	if t == nil {
		return
	}

	var deltaIn, deltaOut int
	if dir == network.DirInbound {
		deltaIn = 1
	} else {
		deltaOut = 1
	}

	t.push(traceEvt{
		Type:       traceBlockAddStreamEvt,
		Scope:      scope,
		DeltaIn:    deltaIn,
		DeltaOut:   deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

func (t *trace) RemoveStream(scope string, dir network.Direction, nstreamsIn, nstreamsOut int) {
	if t == nil {
		return
	}

	var deltaIn, deltaOut int
	if dir == network.DirInbound {
		deltaIn = -1
	} else {
		deltaOut = -1
	}

	t.push(traceEvt{
		Type:       traceRemoveStreamEvt,
		Scope:      scope,
		DeltaIn:    deltaIn,
		DeltaOut:   deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

func (t *trace) AddStreams(scope string, deltaIn, deltaOut, nstreamsIn, nstreamsOut int) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:       traceAddStreamEvt,
		Scope:      scope,
		DeltaIn:    deltaIn,
		DeltaOut:   deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

func (t *trace) BlockAddStreams(scope string, deltaIn, deltaOut, nstreamsIn, nstreamsOut int) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:       traceBlockAddStreamEvt,
		Scope:      scope,
		DeltaIn:    deltaIn,
		DeltaOut:   deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

func (t *trace) RemoveStreams(scope string, deltaIn, deltaOut, nstreamsIn, nstreamsOut int) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:       traceRemoveStreamEvt,
		Scope:      scope,
		DeltaIn:    -deltaIn,
		DeltaOut:   -deltaOut,
		StreamsIn:  nstreamsIn,
		StreamsOut: nstreamsOut,
	})
}

func (t *trace) AddConn(scope string, dir network.Direction, usefd bool, nconnsIn, nconnsOut, nfd int) {
	if t == nil {
		return
	}

	var deltaIn, deltaOut, deltafd int
	if dir == network.DirInbound {
		deltaIn = 1
	} else {
		deltaOut = 1
	}
	if usefd {
		deltafd = 1
	}

	t.push(traceEvt{
		Type:     traceAddConnEvt,
		Scope:    scope,
		DeltaIn:  deltaIn,
		DeltaOut: deltaOut,
		Delta:    int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}

func (t *trace) BlockAddConn(scope string, dir network.Direction, usefd bool, nconnsIn, nconnsOut, nfd int) {
	if t == nil {
		return
	}

	var deltaIn, deltaOut, deltafd int
	if dir == network.DirInbound {
		deltaIn = 1
	} else {
		deltaOut = 1
	}
	if usefd {
		deltafd = 1
	}

	t.push(traceEvt{
		Type:     traceBlockAddConnEvt,
		Scope:    scope,
		DeltaIn:  deltaIn,
		DeltaOut: deltaOut,
		Delta:    int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}

func (t *trace) RemoveConn(scope string, dir network.Direction, usefd bool, nconnsIn, nconnsOut, nfd int) {
	if t == nil {
		return
	}

	var deltaIn, deltaOut, deltafd int
	if dir == network.DirInbound {
		deltaIn = -1
	} else {
		deltaOut = -1
	}
	if usefd {
		deltafd = -1
	}

	t.push(traceEvt{
		Type:     traceRemoveConnEvt,
		Scope:    scope,
		DeltaIn:  deltaIn,
		DeltaOut: deltaOut,
		Delta:    int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}

func (t *trace) AddConns(scope string, deltaIn, deltaOut, deltafd, nconnsIn, nconnsOut, nfd int) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:     traceAddConnEvt,
		Scope:    scope,
		DeltaIn:  deltaIn,
		DeltaOut: deltaOut,
		Delta:    int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}

func (t *trace) BlockAddConns(scope string, deltaIn, deltaOut, deltafd, nconnsIn, nconnsOut, nfd int) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:     traceBlockAddConnEvt,
		Scope:    scope,
		DeltaIn:  deltaIn,
		DeltaOut: deltaOut,
		Delta:    int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}

func (t *trace) RemoveConns(scope string, deltaIn, deltaOut, deltafd, nconnsIn, nconnsOut, nfd int) {
	if t == nil {
		return
	}

	t.push(traceEvt{
		Type:     traceRemoveConnEvt,
		Scope:    scope,
		DeltaIn:  -deltaIn,
		DeltaOut: -deltaOut,
		Delta:    -int64(deltafd),
		ConnsIn:  nconnsIn,
		ConnsOut: nconnsOut,
		FD:       nfd,
	})
}
