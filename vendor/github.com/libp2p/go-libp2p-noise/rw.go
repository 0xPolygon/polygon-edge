package noise

import (
	"github.com/libp2p/go-libp2p/p2p/security/noise"
)

// MaxTransportMsgLength is the Noise-imposed maximum transport message length,
// inclusive of the MAC size (16 bytes, Poly1305 for noise-libp2p).
// Deprecated: use github.com/libp2p/go-libp2p/p2p/security/noise.MaxTransportMsgLength instead.
const MaxTransportMsgLength = noise.MaxTransportMsgLength

// MaxPlaintextLength is the maximum payload size. It is MaxTransportMsgLength
// minus the MAC size. Payloads over this size will be automatically chunked.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/security/noise.MaxPlaintextLength instead.
const MaxPlaintextLength = noise.MaxPlaintextLength

// LengthPrefixLength is the length of the length prefix itself, which precedes
// all transport messages in order to delimit them. In bytes.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/security/noise.LengthPrefixLength instead.
const LengthPrefixLength = noise.LengthPrefixLength
