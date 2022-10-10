package polybft

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/pbft-consensus"
	"github.com/umbracle/ethgo"
)

// MemDBRecord is an interface that all records in memdb implement
type MemDBRecord interface {
	// Key returns unique identification of a db record
	Key() uint64
}

// StateSyncEvent is a bridge event from the rootchain
type StateSyncEvent struct {
	// ID is the decoded 'index' field from the event
	ID uint64
	// Sender is the decoded 'sender' field from the event
	Sender ethgo.Address
	// Target is the decoded 'target' field from the event
	Target ethgo.Address
	// Data is the decoded 'data' field from the event
	Data []byte
	// Log contains raw data about smart contract event execution
	Log *ethgo.Log
}

func (s *StateSyncEvent) String() string {
	return fmt.Sprintf("Id=%d, Sender=%v, Target=%v", s.ID, s.Sender, s.Target)
}

var _ MemDBRecord = &StateSyncEvent{}

//Key returns the ID field of state sync event which is the unique key in memdb
func (s *StateSyncEvent) Key() uint64 {
	return s.ID
}

// newStateSyncEvent creates an instance of pending state sync event.
func newStateSyncEvent(
	id uint64,
	sender ethgo.Address,
	target ethgo.Address,
	data []byte, log *ethgo.Log,
) *StateSyncEvent {
	return &StateSyncEvent{
		ID:     id,
		Sender: sender,
		Target: target,
		Data:   data,
		Log:    log,
	}
}

func decodeEvent(log *ethgo.Log) (*StateSyncEvent, error) {
	raw, err := stateTransferEvent.ParseLog(log)
	if err != nil {
		return nil, err
	}

	id, ok := raw["id"].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("failed to decode id field of log: %+v", log)
	}

	sender, ok := raw["sender"].(ethgo.Address)
	if !ok {
		return nil, fmt.Errorf("failed to decode sender field of log: %+v", log)
	}

	target, ok := raw["target"].(ethgo.Address)
	if !ok {
		return nil, fmt.Errorf("failed to decode target field of log: %+v", log)
	}

	data, ok := raw["data"].([]byte)
	if !ok {
		return nil, fmt.Errorf("failed to decode data field of log: %+v", log)
	}

	return newStateSyncEvent(id.Uint64(), sender, target, data, log), nil
}

// MessageSignature encapsulates sender identifier and its signature
type MessageSignature struct {
	// Signer of the vote
	From pbft.NodeID
	// Signature of the message
	Signature []byte
}

// TransportMessage represents the payload which is gossiped across the network
type TransportMessage struct {
	// Hash is encoded data
	Hash string
	// Message signature
	Signature []byte
	// Node identifier
	NodeID pbft.NodeID
	// Number of epoch
	EpochNumber uint64
}

var _ MemDBRecord = &CommitmentToExecute{}

// CommitmentToExecute represents a finalized commitment
type CommitmentToExecute struct {
	SignedCommitment *CommitmentMessageSigned
	Proofs           []*BundleProof
	ToIndex          uint64
}

// Key returns the ToIndex of commitment which is the unique key in memdb
func (c *CommitmentToExecute) Key() uint64 {
	return c.ToIndex
}

// getBundles returns bundles from commitment that can be executed in current sprint
func (c *CommitmentToExecute) getBundles(maxNumberOfBundles, processedBundles int) ([]*BundleProof, int) {
	commitmentBundlesNum := len(c.Proofs)
	if commitmentBundlesNum == 0 {
		return nil, commitmentBundlesNum
	}

	if (processedBundles + commitmentBundlesNum) < maxNumberOfBundles {
		return c.Proofs, commitmentBundlesNum
	}

	return c.Proofs[0 : maxNumberOfBundles-processedBundles], commitmentBundlesNum
}

var _ MemDBRecord = &MessageVotes{}

// MessageVotes represents accumulated votes for a hash in a single epoch
type MessageVotes struct {
	Epoch      uint64
	Hash       string
	Signatures []*MessageSignature
}

// Key returns the Epoch of message vote which is the unique key in memdb
func (mv *MessageVotes) Key() uint64 {
	return mv.Epoch
}

var _ MemDBRecord = &ValidatorSnapshot{}

// ValidatorSnapshot represents a snapshot of validators for given epoch
type ValidatorSnapshot struct {
	Epoch      uint64
	AccountSet AccountSet
}

// Key returns the Epoch of validator snapshot which is the unique key in memdb
func (vs *ValidatorSnapshot) Key() uint64 {
	return vs.Epoch
}
