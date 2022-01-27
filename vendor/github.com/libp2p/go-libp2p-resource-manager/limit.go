package rcmgr

import (
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
)

// Limit is an object that specifies basic resource limits.
type Limit interface {
	// GetMemoryLimit returns the (current) memory limit.
	GetMemoryLimit() int64
	// GetStreamLimit returns the stream limit, for inbound or outbound streams.
	GetStreamLimit(network.Direction) int
	// GetStreamTotalLimit returns the total stream limit
	GetStreamTotalLimit() int
	// GetConnLimit returns the connection limit, for inbound or outbound connections.
	GetConnLimit(network.Direction) int
	// GetConnTotalLimit returns the total connection limit
	GetConnTotalLimit() int
	// GetFDLimit returns the file descriptor limit.
	GetFDLimit() int

	// WithMemoryLimit creates a copy of this limit object, with memory limit adjusted to
	// the specified memFraction of its current value, bounded by minMemory and maxMemory.
	WithMemoryLimit(memFraction float64, minMemory, maxMemory int64) Limit
	// WithStreamLimit creates a copy of this limit object, with stream limits adjusted
	// as specified.
	WithStreamLimit(numStreamsIn, numStreamsOut, numStreams int) Limit
	// WithConnLimit creates a copy of this limit object, with connetion limits adjusted
	// as specified.
	WithConnLimit(numConnsIn, numConnsOut, numConns int) Limit
	// WithFDLimit creates a copy of this limit object, with file descriptor limits adjusted
	// as specified
	WithFDLimit(numFD int) Limit
}

// Limiter is the interface for providing limits to the resource manager.
type Limiter interface {
	GetSystemLimits() Limit
	GetTransientLimits() Limit
	GetServiceLimits(svc string) Limit
	GetServicePeerLimits(svc string) Limit
	GetProtocolLimits(proto protocol.ID) Limit
	GetProtocolPeerLimits(proto protocol.ID) Limit
	GetPeerLimits(p peer.ID) Limit
	GetStreamLimits(p peer.ID) Limit
	GetConnLimits() Limit
}

// BasicLimiter is a limiter with fixed limits.
type BasicLimiter struct {
	SystemLimits              Limit
	TransientLimits           Limit
	DefaultServiceLimits      Limit
	DefaultServicePeerLimits  Limit
	ServiceLimits             map[string]Limit
	ServicePeerLimits         map[string]Limit
	DefaultProtocolLimits     Limit
	DefaultProtocolPeerLimits Limit
	ProtocolLimits            map[protocol.ID]Limit
	ProtocolPeerLimits        map[protocol.ID]Limit
	DefaultPeerLimits         Limit
	PeerLimits                map[peer.ID]Limit
	ConnLimits                Limit
	StreamLimits              Limit
}

var _ Limiter = (*BasicLimiter)(nil)

// BaseLimit is a mixin type for basic resource limits.
type BaseLimit struct {
	Streams         int
	StreamsInbound  int
	StreamsOutbound int
	Conns           int
	ConnsInbound    int
	ConnsOutbound   int
	FD              int
}

// MemoryLimit is a mixin type for memory limits
type MemoryLimit struct {
	MemoryFraction float64
	MinMemory      int64
	MaxMemory      int64
}

func (l *BaseLimit) GetStreamLimit(dir network.Direction) int {
	if dir == network.DirInbound {
		return l.StreamsInbound
	} else {
		return l.StreamsOutbound
	}
}

func (l *BaseLimit) GetStreamTotalLimit() int {
	return l.Streams
}

func (l *BaseLimit) GetConnLimit(dir network.Direction) int {
	if dir == network.DirInbound {
		return l.ConnsInbound
	} else {
		return l.ConnsOutbound
	}
}

func (l *BaseLimit) GetConnTotalLimit() int {
	return l.Conns
}

func (l *BaseLimit) GetFDLimit() int {
	return l.FD
}

func (l *BasicLimiter) GetSystemLimits() Limit {
	return l.SystemLimits
}

func (l *BasicLimiter) GetTransientLimits() Limit {
	return l.TransientLimits
}

func (l *BasicLimiter) GetServiceLimits(svc string) Limit {
	sl, ok := l.ServiceLimits[svc]
	if !ok {
		return l.DefaultServiceLimits
	}
	return sl
}

func (l *BasicLimiter) GetServicePeerLimits(svc string) Limit {
	pl, ok := l.ServicePeerLimits[svc]
	if !ok {
		return l.DefaultServicePeerLimits
	}
	return pl
}

func (l *BasicLimiter) GetProtocolLimits(proto protocol.ID) Limit {
	pl, ok := l.ProtocolLimits[proto]
	if !ok {
		return l.DefaultProtocolLimits
	}
	return pl
}

func (l *BasicLimiter) GetProtocolPeerLimits(proto protocol.ID) Limit {
	pl, ok := l.ProtocolPeerLimits[proto]
	if !ok {
		return l.DefaultProtocolPeerLimits
	}
	return pl
}

func (l *BasicLimiter) GetPeerLimits(p peer.ID) Limit {
	pl, ok := l.PeerLimits[p]
	if !ok {
		return l.DefaultPeerLimits
	}
	return pl
}

func (l *BasicLimiter) GetStreamLimits(p peer.ID) Limit {
	return l.StreamLimits
}

func (l *BasicLimiter) GetConnLimits() Limit {
	return l.ConnLimits
}

func (l *MemoryLimit) GetMemory(memoryCap int64) int64 {
	return memoryLimit(memoryCap, l.MemoryFraction, l.MinMemory, l.MaxMemory)
}

func memoryLimit(memoryCap int64, memFraction float64, minMemory, maxMemory int64) int64 {
	memoryCap = int64(float64(memoryCap) * memFraction)
	switch {
	case memoryCap < minMemory:
		return minMemory
	case memoryCap > maxMemory:
		return maxMemory
	default:
		return memoryCap
	}
}
