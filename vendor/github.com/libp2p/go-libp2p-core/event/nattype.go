package event

import (
	"github.com/libp2p/go-libp2p/core/event"
)

// EvtNATDeviceTypeChanged is an event struct to be emitted when the type of the NAT device changes for a Transport Protocol.
//
// Note: This event is meaningful ONLY if the AutoNAT Reachability is Private.
// Consumers of this event should ALSO consume the `EvtLocalReachabilityChanged` event and interpret
// this event ONLY if the Reachability on the `EvtLocalReachabilityChanged` is Private.
// Deprecated: use github.com/libp2p/go-libp2p/core/event.EvtNATDeviceTypeChanged instead
type EvtNATDeviceTypeChanged = event.EvtNATDeviceTypeChanged
