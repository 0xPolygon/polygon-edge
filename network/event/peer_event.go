package event

import "github.com/libp2p/go-libp2p-core/peer"

type PeerEventType uint

const (
	PeerConnected        PeerEventType = iota // Emitted when a peer connected
	PeerFailedToConnect                       // Emitted when a peer failed to connect
	PeerDisconnected                          // Emitted when a peer disconnected from node
	PeerDialCompleted                         // Emitted when a peer completed dial
	PeerAddedToDialQueue                      // Emitted when a peer is added to dial queue
)

var peerEventToName = map[PeerEventType]string{
	PeerConnected:        "PeerConnected",
	PeerFailedToConnect:  "PeerFailedToConnect",
	PeerDisconnected:     "PeerDisconnected",
	PeerDialCompleted:    "PeerDialCompleted",
	PeerAddedToDialQueue: "PeerAddedToDialQueue",
}

type PeerEvent struct {
	// PeerID is the id of the peer that triggered
	// the event
	PeerID peer.ID

	// Type is the type of the event
	Type PeerEventType
}

func (s PeerEventType) String() string {
	name, ok := peerEventToName[s]
	if !ok {
		return "unknown"
	}

	return name
}
