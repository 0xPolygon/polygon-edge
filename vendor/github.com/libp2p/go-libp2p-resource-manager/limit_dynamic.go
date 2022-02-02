package rcmgr

import (
	"runtime"

	"github.com/pbnjay/memory"
)

// DynamicLimit is a limit with dynamic memory values, based on available (free) memory
type DynamicLimit struct {
	BaseLimit
	MemoryLimit
}

var _ Limit = (*DynamicLimit)(nil)

func (l *DynamicLimit) GetMemoryLimit() int64 {
	freemem := memory.FreeMemory()

	// account for memory retained by the runtime that is actually free
	// HeapInuse - HeapAlloc is the memory available in allocator spans
	// HeapIdle - HeapReleased is memory held by the runtime that could be returned to the OS
	var memstat runtime.MemStats
	runtime.ReadMemStats(&memstat)

	freemem += (memstat.HeapInuse - memstat.HeapAlloc) + (memstat.HeapIdle - memstat.HeapReleased)
	return l.MemoryLimit.GetMemory(int64(freemem))
}

func (l *DynamicLimit) WithMemoryLimit(memFraction float64, minMemory, maxMemory int64) Limit {
	r := new(DynamicLimit)
	*r = *l

	r.MemoryLimit.MemoryFraction *= memFraction
	r.MemoryLimit.MinMemory = minMemory
	r.MemoryLimit.MaxMemory = maxMemory

	return r
}

func (l *DynamicLimit) WithStreamLimit(numStreamsIn, numStreamsOut, numStreams int) Limit {
	r := new(DynamicLimit)
	*r = *l

	r.BaseLimit.StreamsInbound = numStreamsIn
	r.BaseLimit.StreamsOutbound = numStreamsOut
	r.BaseLimit.Streams = numStreams

	return r
}

func (l *DynamicLimit) WithConnLimit(numConnsIn, numConnsOut, numConns int) Limit {
	r := new(DynamicLimit)
	*r = *l

	r.BaseLimit.ConnsInbound = numConnsIn
	r.BaseLimit.ConnsOutbound = numConnsOut
	r.BaseLimit.Conns = numConns

	return r
}

func (l *DynamicLimit) WithFDLimit(numFD int) Limit {
	r := new(DynamicLimit)
	*r = *l

	r.BaseLimit.FD = numFD

	return r
}

// NewDefaultDynamicLimiter creates a limiter with default limits and a memory cap
// dynamically computed based on available memory.
func NewDefaultDynamicLimiter(memFraction float64, minMemory, maxMemory int64) *BasicLimiter {
	return NewDynamicLimiter(DefaultLimits.WithSystemMemory(memFraction, minMemory, maxMemory))
}

// NewDynamicLimiter crates a dynamic limiter with the specified defaults
func NewDynamicLimiter(cfg DefaultLimitConfig) *BasicLimiter {
	system := &DynamicLimit{
		MemoryLimit: cfg.SystemMemory,
		BaseLimit:   cfg.SystemBaseLimit,
	}
	transient := &DynamicLimit{
		MemoryLimit: cfg.TransientMemory,
		BaseLimit:   cfg.TransientBaseLimit,
	}
	svc := &DynamicLimit{
		MemoryLimit: cfg.ServiceMemory,
		BaseLimit:   cfg.ServiceBaseLimit,
	}
	svcPeer := &DynamicLimit{
		MemoryLimit: cfg.ServicePeerMemory,
		BaseLimit:   cfg.ServicePeerBaseLimit,
	}
	proto := &DynamicLimit{
		MemoryLimit: cfg.ProtocolMemory,
		BaseLimit:   cfg.ProtocolBaseLimit,
	}
	protoPeer := &DynamicLimit{
		MemoryLimit: cfg.ProtocolPeerMemory,
		BaseLimit:   cfg.ProtocolPeerBaseLimit,
	}
	peer := &DynamicLimit{
		MemoryLimit: cfg.PeerMemory,
		BaseLimit:   cfg.PeerBaseLimit,
	}
	conn := &StaticLimit{
		Memory:    cfg.ConnMemory,
		BaseLimit: cfg.ConnBaseLimit,
	}
	stream := &StaticLimit{
		Memory:    cfg.StreamMemory,
		BaseLimit: cfg.StreamBaseLimit,
	}

	return &BasicLimiter{
		SystemLimits:              system,
		TransientLimits:           transient,
		DefaultServiceLimits:      svc,
		DefaultServicePeerLimits:  svcPeer,
		DefaultProtocolLimits:     proto,
		DefaultProtocolPeerLimits: protoPeer,
		DefaultPeerLimits:         peer,
		ConnLimits:                conn,
		StreamLimits:              stream,
	}
}
