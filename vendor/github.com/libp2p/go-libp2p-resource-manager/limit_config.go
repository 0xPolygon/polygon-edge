package rcmgr

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/pbnjay/memory"
)

type BasicLimitConfig struct {
	// if true, then a dynamic limit is used
	Dynamic bool `json:",omitempty"`
	// either Memory is set for fixed memory limit
	Memory int64 `json:",omitempty"`
	// or the following 3 fields for computed memory limits
	MinMemory      int64   `json:",omitempty"`
	MaxMemory      int64   `json:",omitempty"`
	MemoryFraction float64 `json:",omitempty"`

	StreamsInbound  int
	StreamsOutbound int
	Streams         int

	ConnsInbound  int
	ConnsOutbound int
	Conns         int

	FD int
}

func (cfg *BasicLimitConfig) toLimit(base BaseLimit, mem MemoryLimit) (Limit, error) {
	if cfg == nil {
		m := mem.GetMemory(int64(memory.TotalMemory()))
		return &StaticLimit{
			Memory:    m,
			BaseLimit: base,
		}, nil
	}

	if cfg.Streams > 0 {
		base.Streams = cfg.Streams
	}
	if cfg.StreamsInbound > 0 {
		base.StreamsInbound = cfg.StreamsInbound
	}
	if cfg.StreamsOutbound > 0 {
		base.StreamsOutbound = cfg.StreamsOutbound
	}
	if cfg.Conns > 0 {
		base.Conns = cfg.Conns
	}
	if cfg.ConnsInbound > 0 {
		base.ConnsInbound = cfg.ConnsInbound
	}
	if cfg.ConnsOutbound > 0 {
		base.ConnsOutbound = cfg.ConnsOutbound
	}
	if cfg.FD > 0 {
		base.FD = cfg.FD
	}

	switch {
	case cfg.Memory > 0:
		return &StaticLimit{
			Memory:    cfg.Memory,
			BaseLimit: base,
		}, nil

	case cfg.Dynamic:
		if cfg.MemoryFraction < 0 {
			return nil, fmt.Errorf("negative memory fraction: %f", cfg.MemoryFraction)
		}
		if cfg.MemoryFraction > 0 {
			mem.MemoryFraction = cfg.MemoryFraction
		}
		if cfg.MinMemory > 0 {
			mem.MinMemory = cfg.MinMemory
		}
		if cfg.MaxMemory > 0 {
			mem.MaxMemory = cfg.MaxMemory
		}

		return &DynamicLimit{
			MemoryLimit: mem,
			BaseLimit:   base,
		}, nil

	default:
		if cfg.MemoryFraction < 0 {
			return nil, fmt.Errorf("negative memory fraction: %f", cfg.MemoryFraction)
		}
		if cfg.MemoryFraction > 0 {
			mem.MemoryFraction = cfg.MemoryFraction
		}
		if cfg.MinMemory > 0 {
			mem.MinMemory = cfg.MinMemory
		}
		if cfg.MaxMemory > 0 {
			mem.MaxMemory = cfg.MaxMemory
		}

		m := mem.GetMemory(int64(memory.TotalMemory()))
		return &StaticLimit{
			Memory:    m,
			BaseLimit: base,
		}, nil
	}
}

func (cfg *BasicLimitConfig) toLimitFixed(base BaseLimit, mem int64) (Limit, error) {
	if cfg == nil {
		return &StaticLimit{
			Memory:    mem,
			BaseLimit: base,
		}, nil
	}

	if cfg.Streams > 0 {
		base.Streams = cfg.Streams
	}
	if cfg.StreamsInbound > 0 {
		base.StreamsInbound = cfg.StreamsInbound
	}
	if cfg.StreamsOutbound > 0 {
		base.StreamsOutbound = cfg.StreamsOutbound
	}
	if cfg.Conns > 0 {
		base.Conns = cfg.Conns
	}
	if cfg.ConnsInbound > 0 {
		base.ConnsInbound = cfg.ConnsInbound
	}
	if cfg.ConnsOutbound > 0 {
		base.ConnsOutbound = cfg.ConnsOutbound
	}
	if cfg.FD > 0 {
		base.FD = cfg.FD
	}

	switch {
	case cfg.Memory > 0:
		return &StaticLimit{
			Memory:    cfg.Memory,
			BaseLimit: base,
		}, nil

	case cfg.Dynamic:
		return nil, fmt.Errorf("cannot specify dynamic limit for fixed memory limit")

	default:
		if cfg.MemoryFraction > 0 || cfg.MinMemory > 0 || cfg.MaxMemory > 0 {
			return nil, fmt.Errorf("cannot specify dynamic range for fixed memory limit")
		}
		return &StaticLimit{
			Memory:    mem,
			BaseLimit: base,
		}, nil
	}
}

type BasicLimiterConfig struct {
	System    *BasicLimitConfig `json:",omitempty"`
	Transient *BasicLimitConfig `json:",omitempty"`

	ServiceDefault     *BasicLimitConfig           `json:",omitempty"`
	ServicePeerDefault *BasicLimitConfig           `json:",omitempty"`
	Service            map[string]BasicLimitConfig `json:",omitempty"`
	ServicePeer        map[string]BasicLimitConfig `json:",omitempty"`

	ProtocolDefault     *BasicLimitConfig           `json:",omitempty"`
	ProtocolPeerDefault *BasicLimitConfig           `json:",omitempty"`
	Protocol            map[string]BasicLimitConfig `json:",omitempty"`
	ProtocolPeer        map[string]BasicLimitConfig `json:",omitempty"`

	PeerDefault *BasicLimitConfig           `json:",omitempty"`
	Peer        map[string]BasicLimitConfig `json:",omitempty"`

	Conn   *BasicLimitConfig `json:",omitempty"`
	Stream *BasicLimitConfig `json:",omitempty"`
}

// NewDefaultLimiterFromJSON creates a new limiter by parsing a json configuration,
// using the default limits for fallback.
func NewDefaultLimiterFromJSON(in io.Reader) (*BasicLimiter, error) {
	return NewLimiterFromJSON(in, DefaultLimits)
}

// NewLimiterFromJSON creates a new limiter by parsing a json configuration.
func NewLimiterFromJSON(in io.Reader, defaults DefaultLimitConfig) (*BasicLimiter, error) {
	jin := json.NewDecoder(in)

	var cfg BasicLimiterConfig

	if err := jin.Decode(&cfg); err != nil {
		return nil, err
	}

	return NewLimiter(cfg, defaults)
}

func NewLimiter(cfg BasicLimiterConfig, defaults DefaultLimitConfig) (*BasicLimiter, error) {
	limiter := new(BasicLimiter)
	var err error

	limiter.SystemLimits, err = cfg.System.toLimit(defaults.SystemBaseLimit, defaults.SystemMemory)
	if err != nil {
		return nil, fmt.Errorf("invalid system limit: %w", err)
	}

	limiter.TransientLimits, err = cfg.Transient.toLimit(defaults.TransientBaseLimit, defaults.TransientMemory)
	if err != nil {
		return nil, fmt.Errorf("invalid transient limit: %w", err)
	}

	limiter.DefaultServiceLimits, err = cfg.ServiceDefault.toLimit(defaults.ServiceBaseLimit, defaults.ServiceMemory)
	if err != nil {
		return nil, fmt.Errorf("invalid default service limit: %w", err)
	}

	limiter.DefaultServicePeerLimits, err = cfg.ServicePeerDefault.toLimit(defaults.ServicePeerBaseLimit, defaults.ServicePeerMemory)
	if err != nil {
		return nil, fmt.Errorf("invalid default service peer limit: %w", err)
	}

	if len(cfg.Service) > 0 {
		limiter.ServiceLimits = make(map[string]Limit, len(cfg.Service))
		for svc, cfgLimit := range cfg.Service {
			limiter.ServiceLimits[svc], err = cfgLimit.toLimit(defaults.ServiceBaseLimit, defaults.ServiceMemory)
			if err != nil {
				return nil, fmt.Errorf("invalid service limit for %s: %w", svc, err)
			}
		}
	}

	if len(cfg.ServicePeer) > 0 {
		limiter.ServicePeerLimits = make(map[string]Limit, len(cfg.ServicePeer))
		for svc, cfgLimit := range cfg.ServicePeer {
			limiter.ServicePeerLimits[svc], err = cfgLimit.toLimit(defaults.ServicePeerBaseLimit, defaults.ServicePeerMemory)
			if err != nil {
				return nil, fmt.Errorf("invalid service peer limit for %s: %w", svc, err)
			}
		}
	}

	limiter.DefaultProtocolLimits, err = cfg.ProtocolDefault.toLimit(defaults.ProtocolBaseLimit, defaults.ProtocolMemory)
	if err != nil {
		return nil, fmt.Errorf("invalid default protocol limit: %w", err)
	}

	limiter.DefaultProtocolPeerLimits, err = cfg.ProtocolPeerDefault.toLimit(defaults.ProtocolPeerBaseLimit, defaults.ProtocolPeerMemory)
	if err != nil {
		return nil, fmt.Errorf("invalid default protocol peer limit: %w", err)
	}

	if len(cfg.Protocol) > 0 {
		limiter.ProtocolLimits = make(map[protocol.ID]Limit, len(cfg.Protocol))
		for p, cfgLimit := range cfg.Protocol {
			limiter.ProtocolLimits[protocol.ID(p)], err = cfgLimit.toLimit(defaults.ProtocolBaseLimit, defaults.ProtocolMemory)
			if err != nil {
				return nil, fmt.Errorf("invalid service limit for %s: %w", p, err)
			}
		}
	}

	if len(cfg.ProtocolPeer) > 0 {
		limiter.ProtocolPeerLimits = make(map[protocol.ID]Limit, len(cfg.ProtocolPeer))
		for p, cfgLimit := range cfg.ProtocolPeer {
			limiter.ProtocolPeerLimits[protocol.ID(p)], err = cfgLimit.toLimit(defaults.ProtocolPeerBaseLimit, defaults.ProtocolPeerMemory)
			if err != nil {
				return nil, fmt.Errorf("invalid service peer limit for %s: %w", p, err)
			}
		}
	}

	limiter.DefaultPeerLimits, err = cfg.PeerDefault.toLimit(defaults.PeerBaseLimit, defaults.PeerMemory)
	if err != nil {
		return nil, fmt.Errorf("invalid peer limit: %w", err)
	}

	if len(cfg.Peer) > 0 {
		limiter.PeerLimits = make(map[peer.ID]Limit, len(cfg.Peer))
		for p, cfgLimit := range cfg.Peer {
			pid, err := peer.Decode(p)
			if err != nil {
				return nil, fmt.Errorf("invalid peer ID %s: %w", p, err)
			}
			limiter.PeerLimits[pid], err = cfgLimit.toLimit(defaults.PeerBaseLimit, defaults.PeerMemory)
			if err != nil {
				return nil, fmt.Errorf("invalid peer limit for %s: %w", p, err)
			}
		}
	}

	limiter.ConnLimits, err = cfg.Conn.toLimitFixed(defaults.ConnBaseLimit, defaults.ConnMemory)
	if err != nil {
		return nil, fmt.Errorf("invalid conn limit: %w", err)
	}

	limiter.StreamLimits, err = cfg.Stream.toLimitFixed(defaults.StreamBaseLimit, defaults.StreamMemory)
	if err != nil {
		return nil, fmt.Errorf("invalid stream limit: %w", err)
	}

	return limiter, nil
}
