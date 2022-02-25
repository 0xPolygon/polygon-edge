package rcmgr

import (
	"github.com/pbnjay/memory"
)

// StaticLimit is a limit with static values.
type StaticLimit struct {
	BaseLimit
	Memory int64
}

var _ Limit = (*StaticLimit)(nil)

func (l *StaticLimit) GetMemoryLimit() int64 {
	return l.Memory
}

func (l *StaticLimit) WithMemoryLimit(memFraction float64, minMemory, maxMemory int64) Limit {
	r := new(StaticLimit)
	*r = *l

	r.Memory = int64(memFraction * float64(r.Memory))
	if r.Memory < minMemory {
		r.Memory = minMemory
	} else if r.Memory > maxMemory {
		r.Memory = maxMemory
	}

	return r
}

func (l *StaticLimit) WithStreamLimit(numStreamsIn, numStreamsOut, numStreams int) Limit {
	r := new(StaticLimit)
	*r = *l

	r.BaseLimit.StreamsInbound = numStreamsIn
	r.BaseLimit.StreamsOutbound = numStreamsOut
	r.BaseLimit.Streams = numStreams

	return r
}

func (l *StaticLimit) WithConnLimit(numConnsIn, numConnsOut, numConns int) Limit {
	r := new(StaticLimit)
	*r = *l

	r.BaseLimit.ConnsInbound = numConnsIn
	r.BaseLimit.ConnsOutbound = numConnsOut
	r.BaseLimit.Conns = numConns

	return r
}

func (l *StaticLimit) WithFDLimit(numFD int) Limit {
	r := new(StaticLimit)
	*r = *l

	r.BaseLimit.FD = numFD

	return r
}

// NewDefaultStaticLimiter creates a static limiter with default base limits and a system memory cap
// specified as a fraction of total system memory. The assigned memory will not be less than
// minMemory or more than maxMemory.
func NewDefaultStaticLimiter(memFraction float64, minMemory, maxMemory int64) *BasicLimiter {
	return NewStaticLimiter(DefaultLimits.WithSystemMemory(memFraction, minMemory, maxMemory))
}

// NewDefaultFixedLimiter creates a static limiter with default base limits and a specified system
// memory cap.
func NewDefaultFixedLimiter(memoryCap int64) *BasicLimiter {
	return NewStaticLimiter(DefaultLimits.WithSystemMemory(1, memoryCap, memoryCap))
}

// NewDefaultLimiter creates a static limiter with the default limits
func NewDefaultLimiter() *BasicLimiter {
	return NewStaticLimiter(DefaultLimits)
}

// NewStaticLimiter creates a static limiter using the specified system memory cap and default
// limit config.
func NewStaticLimiter(cfg DefaultLimitConfig) *BasicLimiter {
	memoryCap := memoryLimit(
		int64(memory.TotalMemory()),
		cfg.SystemMemory.MemoryFraction,
		cfg.SystemMemory.MinMemory,
		cfg.SystemMemory.MaxMemory)
	system := &StaticLimit{
		Memory:    memoryCap,
		BaseLimit: cfg.SystemBaseLimit,
	}
	transient := &StaticLimit{
		Memory:    cfg.TransientMemory.GetMemory(memoryCap),
		BaseLimit: cfg.TransientBaseLimit,
	}
	svc := &StaticLimit{
		Memory:    cfg.ServiceMemory.GetMemory(memoryCap),
		BaseLimit: cfg.ServiceBaseLimit,
	}
	svcPeer := &StaticLimit{
		Memory:    cfg.ServicePeerMemory.GetMemory(memoryCap),
		BaseLimit: cfg.ServicePeerBaseLimit,
	}
	proto := &StaticLimit{
		Memory:    cfg.ProtocolMemory.GetMemory(memoryCap),
		BaseLimit: cfg.ProtocolBaseLimit,
	}
	protoPeer := &StaticLimit{
		Memory:    cfg.ProtocolPeerMemory.GetMemory(memoryCap),
		BaseLimit: cfg.ProtocolPeerBaseLimit,
	}
	peer := &StaticLimit{
		Memory:    cfg.PeerMemory.GetMemory(memoryCap),
		BaseLimit: cfg.PeerBaseLimit,
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
