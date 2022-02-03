package rcmgr

// DefaultLimitConfig is a struct for configuring default limits.
type DefaultLimitConfig struct {
	SystemBaseLimit BaseLimit
	SystemMemory    MemoryLimit

	TransientBaseLimit BaseLimit
	TransientMemory    MemoryLimit

	ServiceBaseLimit BaseLimit
	ServiceMemory    MemoryLimit

	ServicePeerBaseLimit BaseLimit
	ServicePeerMemory    MemoryLimit

	ProtocolBaseLimit BaseLimit
	ProtocolMemory    MemoryLimit

	ProtocolPeerBaseLimit BaseLimit
	ProtocolPeerMemory    MemoryLimit

	PeerBaseLimit BaseLimit
	PeerMemory    MemoryLimit

	ConnBaseLimit BaseLimit
	ConnMemory    int64

	StreamBaseLimit BaseLimit
	StreamMemory    int64
}

func (cfg *DefaultLimitConfig) WithSystemMemory(memFraction float64, minMemory, maxMemory int64) DefaultLimitConfig {
	refactor := memFraction / cfg.SystemMemory.MemoryFraction
	r := *cfg
	r.SystemMemory.MemoryFraction = memFraction
	r.SystemMemory.MinMemory = minMemory
	r.SystemMemory.MaxMemory = maxMemory
	r.TransientMemory.MemoryFraction *= refactor
	r.ServiceMemory.MemoryFraction *= refactor
	r.ServicePeerMemory.MemoryFraction *= refactor
	r.ProtocolMemory.MemoryFraction *= refactor
	r.ProtocolPeerMemory.MemoryFraction *= refactor
	r.PeerMemory.MemoryFraction *= refactor
	return r
}

// DefaultLimits are the limits used by the default limiter constructors.
var DefaultLimits = DefaultLimitConfig{
	SystemBaseLimit: BaseLimit{
		StreamsInbound:  4096,
		StreamsOutbound: 16384,
		Streams:         16384,
		ConnsInbound:    256,
		ConnsOutbound:   1024,
		Conns:           1024,
		FD:              512,
	},

	SystemMemory: MemoryLimit{
		MemoryFraction: 0.125,
		MinMemory:      128 << 20,
		MaxMemory:      1 << 30,
	},

	TransientBaseLimit: BaseLimit{
		StreamsInbound:  128,
		StreamsOutbound: 512,
		Streams:         512,
		ConnsInbound:    32,
		ConnsOutbound:   128,
		Conns:           128,
		FD:              128,
	},

	TransientMemory: MemoryLimit{
		MemoryFraction: 1,
		MinMemory:      64 << 20,
		MaxMemory:      64 << 20,
	},

	ServiceBaseLimit: BaseLimit{
		StreamsInbound:  2048,
		StreamsOutbound: 8192,
		Streams:         8192,
	},

	ServiceMemory: MemoryLimit{
		MemoryFraction: 0.125 / 4,
		MinMemory:      64 << 20,
		MaxMemory:      256 << 20,
	},

	ServicePeerBaseLimit: BaseLimit{
		StreamsInbound:  256,
		StreamsOutbound: 512,
		Streams:         512,
	},

	ServicePeerMemory: MemoryLimit{
		MemoryFraction: 0.125 / 16,
		MinMemory:      16 << 20,
		MaxMemory:      64 << 20,
	},

	ProtocolBaseLimit: BaseLimit{
		StreamsInbound:  1024,
		StreamsOutbound: 4096,
		Streams:         4096,
	},

	ProtocolMemory: MemoryLimit{
		MemoryFraction: 0.125 / 8,
		MinMemory:      64 << 20,
		MaxMemory:      128 << 20,
	},

	ProtocolPeerBaseLimit: BaseLimit{
		StreamsInbound:  128,
		StreamsOutbound: 256,
		Streams:         512,
	},

	ProtocolPeerMemory: MemoryLimit{
		MemoryFraction: 0.125 / 16,
		MinMemory:      16 << 20,
		MaxMemory:      64 << 20,
	},

	PeerBaseLimit: BaseLimit{
		StreamsInbound:  512,
		StreamsOutbound: 1024,
		Streams:         1024,
		ConnsInbound:    8,
		ConnsOutbound:   16,
		Conns:           16,
		FD:              8,
	},

	PeerMemory: MemoryLimit{
		MemoryFraction: 0.125 / 16,
		MinMemory:      64 << 20,
		MaxMemory:      128 << 20,
	},

	ConnBaseLimit: BaseLimit{
		ConnsInbound:  1,
		ConnsOutbound: 1,
		Conns:         1,
		FD:            1,
	},

	ConnMemory: 1 << 20,

	StreamBaseLimit: BaseLimit{
		StreamsInbound:  1,
		StreamsOutbound: 1,
		Streams:         1,
	},

	StreamMemory: 16 << 20,
}
