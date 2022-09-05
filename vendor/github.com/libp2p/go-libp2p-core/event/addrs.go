package event

import (
	"github.com/libp2p/go-libp2p/core/event"
)

// AddrAction represents an action taken on one of a Host's listen addresses.
// It is used to add context to address change events in EvtLocalAddressesUpdated.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.AddrAction instead
type AddrAction = event.AddrAction

const (
	// Unknown means that the event producer was unable to determine why the address
	// is in the current state.
	// Deprecated: use github.com/libp2p/go-libp2p/core/event.Unknown instead
	Unknown = event.Unknown

	// Added means that the address is new and was not present prior to the event.
	// Deprecated: use github.com/libp2p/go-libp2p/core/event.Added instead
	Added = event.Added

	// Maintained means that the address was not altered between the current and
	// previous states.
	// Deprecated: use github.com/libp2p/go-libp2p/core/event.Maintained instead
	Maintained = event.Maintained

	// Removed means that the address was removed from the Host.
	// Deprecated: use github.com/libp2p/go-libp2p/core/event.Removed instead
	Removed = event.Removed
)

// UpdatedAddress is used in the EvtLocalAddressesUpdated event to convey
// address change information.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.UpdatedAddress instead
type UpdatedAddress = event.UpdatedAddress

// EvtLocalAddressesUpdated should be emitted when the set of listen addresses for
// the local host changes. This may happen for a number of reasons. For example,
// we may have opened a new relay connection, established a new NAT mapping via
// UPnP, or been informed of our observed address by another peer.
//
// EvtLocalAddressesUpdated contains a snapshot of the current listen addresses,
// and may also contain a diff between the current state and the previous state.
// If the event producer is capable of creating a diff, the Diffs field will be
// true, and event consumers can inspect the Action field of each UpdatedAddress
// to see how each address was modified.
//
// For example, the Action will tell you whether an address in
// the Current list was Added by the event producer, or was Maintained without
// changes. Addresses that were removed from the Host will have the AddrAction
// of Removed, and will be in the Removed list.
//
// If the event producer is not capable or producing diffs, the Diffs field will
// be false, the Removed list will always be empty, and the Action for each
// UpdatedAddress in the Current list will be Unknown.
//
// In addition to the above, EvtLocalAddressesUpdated also contains the updated peer.PeerRecord
// for the Current set of listen addresses, wrapped in a record.Envelope and signed by the Host's private key.
// This record can be shared with other peers to inform them of what we believe are our  diallable addresses
// a secure and authenticated way.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.EvtLocalAddressesUpdated instead
type EvtLocalAddressesUpdated = event.EvtLocalAddressesUpdated
