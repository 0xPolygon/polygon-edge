package core

import (
	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
)

// MessageConstructor defines a message constructor interface
type MessageConstructor interface {
	// BuildPrePrepareMessage builds a PREPREPARE message based on the passed in proposal
	BuildPrePrepareMessage(
		proposal []byte,
		certificate *proto.RoundChangeCertificate,
		view *proto.View,
	) *proto.Message

	// BuildPrepareMessage builds a PREPARE message based on the passed in proposal
	BuildPrepareMessage(proposalHash []byte, view *proto.View) *proto.Message

	// BuildCommitMessage builds a COMMIT message based on the passed in proposal
	BuildCommitMessage(proposalHash []byte, view *proto.View) *proto.Message

	// BuildRoundChangeMessage builds a ROUND_CHANGE message based on the passed in proposal
	BuildRoundChangeMessage(
		proposal []byte,
		certificate *proto.PreparedCertificate,
		view *proto.View,
	) *proto.Message
}

// Verifier defines the verifier interface
type Verifier interface {
	// IsValidBlock checks if the proposed block is child of parent
	IsValidBlock(block []byte) bool

	// IsValidSender checks if signature is from sender
	IsValidSender(msg *proto.Message) bool

	// IsProposer checks if the passed in ID is the Proposer for current view (sequence, round)
	IsProposer(id []byte, height, round uint64) bool

	// IsValidProposalHash checks if the hash matches the proposal
	IsValidProposalHash(proposal, hash []byte) bool

	// IsValidCommittedSeal checks if the seal for the proposal is valid
	IsValidCommittedSeal(proposal []byte, committedSeal *messages.CommittedSeal) bool
}

// Backend defines an interface all backend implementations
// need to implement
type Backend interface {
	MessageConstructor
	Verifier

	// BuildProposal builds a new block proposal
	BuildProposal(blockNumber uint64) []byte

	// InsertBlock inserts a proposal with the specified committed seals
	InsertBlock(proposal []byte, committedSeals []*messages.CommittedSeal)

	// ID returns the validator's ID
	ID() []byte

	// MaximumFaultyNodes returns the maximum number of faulty nodes based
	// on the validator set.
	MaximumFaultyNodes() uint64

	// Quorum returns what is the quorum size for the
	// specified block height.
	Quorum(blockHeight uint64) uint64
}
