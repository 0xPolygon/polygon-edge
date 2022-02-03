package libp2p

import (
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/libp2p/go-libp2p/p2p/host/autonat"
	relayv1 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv1/relay"
	circuit "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/proto"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"

	rcmgr "github.com/libp2p/go-libp2p-resource-manager"
)

// SetDefaultServiceLimits sets the default limits for bundled libp2p services
//
// More specifically this sets the following limits:
// - identify:
//   128 streams in, 128 streams out, 256 streams total, 4MB min, 64MB max svc memory
//   16/16/32 streams per peer
// - ping:
//   128 streams in, 128 sreams out, 256 streasms total, 4MB min, 64MB max svc memory
//   2/3/4 streams per peer
// - autonat
//   128 streams in, 128 streams out, 128 streams total, 4MB min, 64MB max svc memory
//   2/2/2 streams per peer
// - holepunch
//   128 streams in, 128 streams out, 128 streams total, 4MB min, 64MB max svc memory
//   2/2/2 streams per peer
// - relay v1 and v2 (separate services)
//   1024 streams in, 1024 streams out, 1024 streams total, 4MB min, 64MB max svc memory
//   64/64/64 streams per peer
func SetDefaultServiceLimits(limiter *rcmgr.BasicLimiter) {
	if limiter.ServiceLimits == nil {
		limiter.ServiceLimits = make(map[string]rcmgr.Limit)
	}
	if limiter.ServicePeerLimits == nil {
		limiter.ServicePeerLimits = make(map[string]rcmgr.Limit)
	}
	if limiter.ProtocolLimits == nil {
		limiter.ProtocolLimits = make(map[protocol.ID]rcmgr.Limit)
	}
	if limiter.ProtocolPeerLimits == nil {
		limiter.ProtocolPeerLimits = make(map[protocol.ID]rcmgr.Limit)
	}

	// identify
	setServiceLimits(limiter, identify.ServiceName,
		limiter.DefaultServiceLimits.
			WithMemoryLimit(1, 4<<20, 64<<20). // max 64MB service memory
			WithStreamLimit(128, 128, 256),    // max 256 streams -- symmetric
		peerLimit(16, 16, 32))

	setProtocolLimits(limiter, identify.ID,
		limiter.DefaultProtocolLimits.WithMemoryLimit(1, 4<<20, 32<<20),
		peerLimit(16, 16, 32))
	setProtocolLimits(limiter, identify.IDPush,
		limiter.DefaultProtocolLimits.WithMemoryLimit(1, 4<<20, 32<<20),
		peerLimit(16, 16, 32))
	setProtocolLimits(limiter, identify.IDDelta,
		limiter.DefaultProtocolLimits.WithMemoryLimit(1, 4<<20, 32<<20),
		peerLimit(16, 16, 32))

	// ping
	setServiceLimits(limiter, ping.ServiceName,
		limiter.DefaultServiceLimits.
			WithMemoryLimit(1, 4<<20, 64<<20). // max 64MB service memory
			WithStreamLimit(128, 128, 128),    // max 128 streams - asymmetric
		peerLimit(2, 3, 4))
	setProtocolLimits(limiter, ping.ID,
		limiter.DefaultProtocolLimits.WithMemoryLimit(1, 4<<20, 64<<20),
		peerLimit(2, 3, 4))

	// autonat
	setServiceLimits(limiter, autonat.ServiceName,
		limiter.DefaultServiceLimits.
			WithMemoryLimit(1, 4<<20, 64<<20). // max 64MB service memory
			WithStreamLimit(128, 128, 128),    // max 128 streams - asymmetric
		peerLimit(2, 2, 2))
	setProtocolLimits(limiter, autonat.AutoNATProto,
		limiter.DefaultProtocolLimits.WithMemoryLimit(1, 4<<20, 64<<20),
		peerLimit(2, 2, 2))

	// holepunch
	setServiceLimits(limiter, holepunch.ServiceName,
		limiter.DefaultServiceLimits.
			WithMemoryLimit(1, 4<<20, 64<<20). // max 64MB service memory
			WithStreamLimit(128, 128, 256),    // max 256 streams - symmetric
		peerLimit(2, 2, 2))
	setProtocolLimits(limiter, holepunch.Protocol,
		limiter.DefaultProtocolLimits.WithMemoryLimit(1, 4<<20, 64<<20),
		peerLimit(2, 2, 2))

	// relay/v1
	setServiceLimits(limiter, relayv1.ServiceName,
		limiter.DefaultServiceLimits.
			WithMemoryLimit(1, 4<<20, 64<<20). // max 64MB service memory
			WithStreamLimit(1024, 1024, 1024), // max 1024 streams - asymmetric
		peerLimit(64, 64, 64))

	// relay/v2
	setServiceLimits(limiter, relayv2.ServiceName,
		limiter.DefaultServiceLimits.
			WithMemoryLimit(1, 4<<20, 64<<20). // max 64MB service memory
			WithStreamLimit(1024, 1024, 1024), // max 1024 streams - asymmetric
		peerLimit(64, 64, 64))

	// circuit protocols, both client and service
	setProtocolLimits(limiter, circuit.ProtoIDv1,
		limiter.DefaultProtocolLimits.
			WithMemoryLimit(1, 4<<20, 64<<20).
			WithStreamLimit(1280, 1280, 1280),
		peerLimit(128, 128, 128))
	setProtocolLimits(limiter, circuit.ProtoIDv2Hop,
		limiter.DefaultProtocolLimits.
			WithMemoryLimit(1, 4<<20, 64<<20).
			WithStreamLimit(1280, 1280, 1280),
		peerLimit(128, 128, 128))
	setProtocolLimits(limiter, circuit.ProtoIDv2Stop,
		limiter.DefaultProtocolLimits.
			WithMemoryLimit(1, 4<<20, 64<<20).
			WithStreamLimit(1280, 1280, 1280),
		peerLimit(128, 128, 128))

}

func setServiceLimits(limiter *rcmgr.BasicLimiter, svc string, limit rcmgr.Limit, peerLimit rcmgr.Limit) {
	if _, ok := limiter.ServiceLimits[svc]; !ok {
		limiter.ServiceLimits[svc] = limit
	}
	if _, ok := limiter.ServicePeerLimits[svc]; !ok {
		limiter.ServicePeerLimits[svc] = peerLimit
	}
}

func setProtocolLimits(limiter *rcmgr.BasicLimiter, proto protocol.ID, limit rcmgr.Limit, peerLimit rcmgr.Limit) {
	if _, ok := limiter.ProtocolLimits[proto]; !ok {
		limiter.ProtocolLimits[proto] = limit
	}
	if _, ok := limiter.ProtocolPeerLimits[proto]; !ok {
		limiter.ProtocolPeerLimits[proto] = peerLimit
	}
}

func peerLimit(numStreamsIn, numStreamsOut, numStreamsTotal int) rcmgr.Limit {
	return &rcmgr.StaticLimit{
		// memory: 256kb for window buffers plus some change for message buffers per stream
		Memory: int64(numStreamsTotal * (256<<10 + 16384)),
		BaseLimit: rcmgr.BaseLimit{
			StreamsInbound:  numStreamsIn,
			StreamsOutbound: numStreamsOut,
			Streams:         numStreamsTotal,
		},
	}
}
