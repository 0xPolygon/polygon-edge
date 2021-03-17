package consensus

/*
// TODO: Remove
// Constants to match up protocol versions and messages
const (
	Eth62 = 62
	Eth63 = 63
)

var (
	EthProtocol = Protocol{
		Name:     "eth",
		Versions: []uint{Eth62, Eth63},
		Lengths:  []uint64{17, 8},
	}
)

// Protocol defines the protocol of the consensus
type Protocol struct {
	// Official short name of the protocol used during capability negotiation.
	Name string
	// Supported versions of the eth protocol (first is primary).
	Versions []uint
	// Number of implemented message corresponding to different protocol versions.
	Lengths []uint64
}

// Broadcaster defines the interface to enqueue blocks to fetcher and find peer
type Broadcaster interface {
	// Enqueue add a block into fetcher queue
	Enqueue(id string, block *types.Block)
	// FindPeers retrives peers by addresses
	FindPeers(map[types.Address]bool) map[types.Address]Peer
}

// Peer defines the interface to communicate with peer
type Peer interface {
	// Send sends the message to this peer
	Send(msgcode uint64, data interface{}) error
}
*/
