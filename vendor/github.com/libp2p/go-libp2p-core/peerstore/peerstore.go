// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/core/peerstore.
//
// Package peerstore provides types and interfaces for local storage of address information,
// metadata, and public key material about libp2p peers.
package peerstore

import (
	"github.com/libp2p/go-libp2p/core/peerstore"
)

// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.ErrNotFound instead
var ErrNotFound = peerstore.ErrNotFound

var (
	// AddressTTL is the expiration time of addresses.
	// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.AddressTTL instead
	AddressTTL = peerstore.AddressTTL

	// TempAddrTTL is the ttl used for a short lived address.
	// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.TempAddrTTL instead
	TempAddrTTL = peerstore.TempAddrTTL

	// ProviderAddrTTL is the TTL of an address we've received from a provider.
	// This is also a temporary address, but lasts longer. After this expires,
	// the records we return will require an extra lookup.
	// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.ProviderAddrTTL instead
	ProviderAddrTTL = peerstore.ProviderAddrTTL

	// RecentlyConnectedAddrTTL is used when we recently connected to a peer.
	// It means that we are reasonably certain of the peer's address.
	// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.RecentlyConnectedAddrTTL instead
	RecentlyConnectedAddrTTL = peerstore.RecentlyConnectedAddrTTL

	// OwnObservedAddrTTL is used for our own external addresses observed by peers.
	// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.OwnObservedAddrTTL instead
	OwnObservedAddrTTL = peerstore.OwnObservedAddrTTL
)

// Permanent TTLs (distinct so we can distinguish between them, constant as they
// are, in fact, permanent)
const (
	// PermanentAddrTTL is the ttl for a "permanent address" (e.g. bootstrap nodes).
	// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.PermanentAddrTTL instead
	PermanentAddrTTL = peerstore.PermanentAddrTTL

	// ConnectedAddrTTL is the ttl used for the addresses of a peer to whom
	// we're connected directly. This is basically permanent, as we will
	// clear them + re-add under a TempAddrTTL after disconnecting.
	// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.ConnectedAddrTTL instead
	ConnectedAddrTTL = peerstore.ConnectedAddrTTL
)

// Peerstore provides a threadsafe store of Peer related
// information.
// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.Peerstore instead
type Peerstore = peerstore.Peerstore

// PeerMetadata can handle values of any type. Serializing values is
// up to the implementation. Dynamic type introspection may not be
// supported, in which case explicitly enlisting types in the
// serializer may be required.
//
// Refer to the docs of the underlying implementation for more
// information.
// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.PeerMetadata instead
type PeerMetadata = peerstore.PeerMetadata

// AddrBook holds the multiaddrs of peers.
// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.AddrBook instead
type AddrBook = peerstore.AddrBook

// CertifiedAddrBook manages "self-certified" addresses for remote peers.
// Self-certified addresses are contained in peer.PeerRecords
// which are wrapped in a record.Envelope and signed by the peer
// to whom they belong.
//
// Certified addresses (CA) are generally more secure than uncertified
// addresses (UA). Consequently, CAs beat and displace UAs. When the
// peerstore learns CAs for a peer, it will reject UAs for the same peer
// (as long as the former haven't expired).
// Furthermore, peer records act like sequenced snapshots of CAs. Therefore,
// processing a peer record that's newer than the last one seen overwrites
// all addresses with the incoming ones.
//
// This interface is most useful when combined with AddrBook.
// To test whether a given AddrBook / Peerstore implementation supports
// certified addresses, callers should use the GetCertifiedAddrBook helper or
// type-assert on the CertifiedAddrBook interface:
//
//	if cab, ok := aPeerstore.(CertifiedAddrBook); ok {
//	    cab.ConsumePeerRecord(signedPeerRecord, aTTL)
//	}
//
// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.CertifiedAddrBook instead
type CertifiedAddrBook = peerstore.CertifiedAddrBook

// GetCertifiedAddrBook is a helper to "upcast" an AddrBook to a
// CertifiedAddrBook by using type assertion. If the given AddrBook
// is also a CertifiedAddrBook, it will be returned, and the ok return
// value will be true. Returns (nil, false) if the AddrBook is not a
// CertifiedAddrBook.
//
// Note that since Peerstore embeds the AddrBook interface, you can also
// call GetCertifiedAddrBook(myPeerstore).
// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.GetCertifiedAddrBook instead
func GetCertifiedAddrBook(ab AddrBook) (cab CertifiedAddrBook, ok bool) {
	return peerstore.GetCertifiedAddrBook(ab)
}

// KeyBook tracks the keys of Peers.
// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.KeyBook instead
type KeyBook = peerstore.KeyBook

// Metrics tracks metrics across a set of peers.
// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.Metrics instead
type Metrics = peerstore.Metrics

// ProtoBook tracks the protocols supported by peers.
// Deprecated: use github.com/libp2p/go-libp2p/core/peerstore.ProtoBook instead
type ProtoBook = peerstore.ProtoBook
