package pbft

// ValidatorSet represents the validator set bahavior
type ValidatorSet interface {
	CalcProposer(round uint64) NodeID
	Includes(id NodeID) bool
	Len() int
}

// Logger represents logger behavior
type Logger interface {
	Printf(format string, args ...interface{})
	Print(args ...interface{})
}

// Transport is a generic interface for a gossip transport protocol
type Transport interface {
	// Gossip broadcast the message to the network
	Gossip(msg *MessageReq) error
}

// SignKey represents the behavior of the signing key
type SignKey interface {
	NodeID() NodeID
	Sign(b []byte) ([]byte, error)
}

// StateNotifier enables custom logic encapsulation related to internal triggers within PBFT state machine (namely receiving timeouts).
type StateNotifier interface {
	// HandleTimeout notifies that a timeout occurred while getting next message
	HandleTimeout(to NodeID, msgType MsgType, view *View)

	// ReadNextMessage reads the next message from message queue of the state machine
	ReadNextMessage(p *Pbft) (*MessageReq, []*MessageReq)
}

// Backend represents the backend behavior
type Backend interface {
	// BuildProposal builds a proposal for the current round (used if proposer)
	BuildProposal() (*Proposal, error)

	// Height returns the height for the current round
	Height() uint64

	// Init is used to signal the backend that a new round is going to start.
	Init(*RoundInfo)

	// Insert inserts the sealed proposal
	Insert(p *SealedProposal) error

	// IsStuck returns whether the pbft is stucked
	IsStuck(num uint64) (uint64, bool)

	// Validate validates a raw proposal (used if non-proposer)
	Validate(*Proposal) error

	// ValidatorSet returns the validator set for the current round
	ValidatorSet() ValidatorSet

	// ValidateCommit is used to validate that a given commit is valid
	ValidateCommit(from NodeID, seal []byte) error
}
