package polybft

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"path"
	"sync"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	polybftProto "github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/tracker"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/umbracle/ethgo"
	"google.golang.org/protobuf/proto"
)

const (
	// number of stateSyncEvents to be processed before a commitment message can be created and gossiped
	stateSyncCommitmentSize = 10
)

// StateSyncManager is an interface that defines functions for state sync workflow
type StateSyncManager interface {
	Init() error
	Commitment() (*CommitmentMessageSigned, error)
	PostBlock(req *PostBlockRequest) error
	PostEpoch(req *PostEpochRequest) error
}

var _ StateSyncManager = (*dummyStateSyncManager)(nil)

// dummyStateSyncManager is used when bridge is not enabled
type dummyStateSyncManager struct{}

func (n *dummyStateSyncManager) Init() error                                   { return nil }
func (n *dummyStateSyncManager) Commitment() (*CommitmentMessageSigned, error) { return nil, nil }
func (n *dummyStateSyncManager) PostBlock(req *PostBlockRequest) error         { return nil }
func (n *dummyStateSyncManager) PostEpoch(req *PostEpochRequest) error         { return nil }

// stateSyncConfig holds the configuration data of state sync manager
type stateSyncConfig struct {
	stateSenderAddr types.Address
	jsonrpcAddr     string
	dataDir         string
	topic           topic
	key             *wallet.Key
}

var _ StateSyncManager = (*stateSyncManager)(nil)

// stateSyncManager is a struct that manages the workflow of
// saving and querying state sync events, and creating, and submitting new commitments
type stateSyncManager struct {
	logger hclog.Logger
	state  *State

	config *stateSyncConfig

	// per epoch fields
	lock         sync.Mutex
	commitment   *Commitment
	epoch        uint64
	validatorSet *validatorSet
}

// topic is an interface for p2p message gossiping
type topic interface {
	Publish(obj proto.Message) error
	Subscribe(handler func(obj interface{}, from peer.ID)) error
}

// NewStateSyncManager creates a new instance of state sync manager
func NewStateSyncManager(logger hclog.Logger, state *State, config *stateSyncConfig) (*stateSyncManager, error) {
	s := &stateSyncManager{
		logger: logger.Named("state-sync"),
		state:  state,
		config: config,
		lock:   sync.Mutex{},
	}

	return s, nil
}

// Init subscribes to bridge topics (getting votes) and start the event tracker routine
func (s *stateSyncManager) Init() error {
	if err := s.initTracker(); err != nil {
		return fmt.Errorf("failed to init event tracker. Error: %w", err)
	}

	if err := s.initTransport(); err != nil {
		return fmt.Errorf("failed to initialize state sync transport layer. Error: %w", err)
	}

	return nil
}

// initTracker starts a new event tracker (to receive new state sync events)
func (s *stateSyncManager) initTracker() error {
	tracker := tracker.NewEventTracker(
		path.Join(s.config.dataDir, "/deposit.db"),
		s.config.jsonrpcAddr,
		ethgo.Address(s.config.stateSenderAddr),
		s,
		s.logger)

	return tracker.Start()
}

// initTransport subscribes to bridge topics (getting votes for commitments)
func (s *stateSyncManager) initTransport() error {
	return s.config.topic.Subscribe(func(obj interface{}, _ peer.ID) {
		msg, ok := obj.(*polybftProto.TransportMessage)
		if !ok {
			s.logger.Warn("failed to deliver vote, invalid msg", "obj", obj)

			return
		}

		var transportMsg *TransportMessage

		if err := json.Unmarshal(msg.Data, &transportMsg); err != nil {
			s.logger.Warn("failed to deliver vote", "error", err)

			return
		}

		if err := s.saveVote(transportMsg); err != nil {
			s.logger.Warn("failed to deliver vote", "error", err)
		}
	})
}

// saveVote saves the gotten vote to boltDb for later quorum check and signature aggregation
func (s *stateSyncManager) saveVote(msg *TransportMessage) error {
	s.lock.Lock()
	epoch := s.epoch
	valSet := s.validatorSet
	s.lock.Unlock()

	if valSet == nil || msg.EpochNumber < epoch {
		// Epoch metadata is undefined or received message for some of the older epochs
		return nil
	}

	if !valSet.Includes(types.StringToAddress(msg.NodeID)) {
		return fmt.Errorf("validator is not among the active validator set")
	}

	msgVote := &MessageSignature{
		From:      msg.NodeID,
		Signature: msg.Signature,
	}

	numSignatures, err := s.state.insertMessageVote(msg.EpochNumber, msg.Hash, msgVote)
	if err != nil {
		return fmt.Errorf("error inserting message vote: %w", err)
	}

	s.logger.Info(
		"deliver message",
		"hash", hex.EncodeToString(msg.Hash),
		"sender", msg.NodeID,
		"signatures", numSignatures,
	)

	return nil
}

// AddLog saves the received log from event tracker if it matches a state sync event ABI
func (s *stateSyncManager) AddLog(eventLog *ethgo.Log) {
	if !stateTransferEventABI.Match(eventLog) {
		return
	}

	s.logger.Info(
		"Add State sync event",
		"block", eventLog.BlockNumber,
		"hash", eventLog.TransactionHash,
		"index", eventLog.LogIndex,
	)

	event, err := decodeStateSyncEvent(eventLog)
	if err != nil {
		s.logger.Error("could not decode state sync event", "err", err)

		return
	}

	if err := s.state.insertStateSyncEvent(event); err != nil {
		s.logger.Error("could not save state sync event to boltDb", "err", err)

		return
	}
}

// Commitment returns a commitment to be submitted if there is a pending commitment with quorum
func (s *stateSyncManager) Commitment() (*CommitmentMessageSigned, error) {
	if s.commitment == nil {
		return nil, nil
	}

	commitment := s.commitment

	commitmentMessage := NewCommitmentMessage(
		commitment.MerkleTree.Hash(),
		commitment.FromIndex,
		commitment.ToIndex)

	aggregatedSignature, publicKeys, err := s.getAggSignatureForCommitmentMessage(commitment)
	if err != nil {
		if errors.Is(err, errQuorumNotReached) {
			// a valid case, commitment has no quorum, we should not return an error
			s.logger.Debug("can not submit a commitment, quorum not reached",
				"from", commitmentMessage.FromIndex,
				"to", commitmentMessage.ToIndex)

			return nil, nil
		}

		return nil, err
	}

	msg := &CommitmentMessageSigned{
		Message:      commitmentMessage,
		AggSignature: aggregatedSignature,
		PublicKeys:   publicKeys,
	}

	return msg, nil
}

// getAggSignatureForCommitmentMessage checks if pending commitment has quorum,
// and if it does, aggregates the signatures
func (s *stateSyncManager) getAggSignatureForCommitmentMessage(commitment *Commitment) (Signature, [][]byte, error) {
	validatorSet := s.validatorSet

	validatorAddrToIndex := make(map[string]int, validatorSet.Len())
	validatorsMetadata := validatorSet.Accounts()

	for i, validator := range validatorsMetadata {
		validatorAddrToIndex[validator.Address.String()] = i
	}

	commitmentHash, err := commitment.Hash()
	if err != nil {
		return Signature{}, nil, err
	}

	// get all the votes from the database for this commitment
	votes, err := s.state.getMessageVotes(commitment.Epoch, commitmentHash.Bytes())
	if err != nil {
		return Signature{}, nil, err
	}

	var signatures bls.Signatures

	publicKeys := make([][]byte, 0)
	bitmap := bitmap.Bitmap{}
	signers := make(map[types.Address]struct{}, 0)

	for _, vote := range votes {
		index, exists := validatorAddrToIndex[vote.From]
		if !exists {
			continue // don't count this vote, because it does not belong to validator
		}

		signature, err := bls.UnmarshalSignature(vote.Signature)
		if err != nil {
			return Signature{}, nil, err
		}

		bitmap.Set(uint64(index))

		signatures = append(signatures, signature)
		publicKeys = append(publicKeys, validatorsMetadata[index].BlsKey.Marshal())
		signers[types.StringToAddress(vote.From)] = struct{}{}
	}

	if !validatorSet.HasQuorum(signers) {
		return Signature{}, nil, errQuorumNotReached
	}

	aggregatedSignature, err := signatures.Aggregate().Marshal()
	if err != nil {
		return Signature{}, nil, err
	}

	result := Signature{
		AggregatedSignature: aggregatedSignature,
		Bitmap:              bitmap,
	}

	return result, publicKeys, nil
}

type PostEpochRequest struct {
	// BlockNumber is the number of the block being executed
	BlockNumber uint64

	// NewEpochID is the id of the new epoch
	NewEpochID uint64

	// SystemState is the state of the governance smart contracts
	// after this block
	SystemState SystemState

	// ValidatorSet is the validator set for the new epoch
	ValidatorSet *validatorSet
}

// PostEpoch notifies the state sync manager that an epoch has changed,
// so that it can discard any previous epoch commitments, and build a new one (since validator set changed)
func (s *stateSyncManager) PostEpoch(req *PostEpochRequest) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// build a new commitment at the end of the epoch
	nextCommittedIndex, err := req.SystemState.GetNextCommittedIndex()
	if err != nil {
		return err
	}

	commitment, err := s.buildCommitment(req.NewEpochID, nextCommittedIndex)
	if err != nil {
		return err
	}

	s.commitment = commitment
	s.validatorSet = req.ValidatorSet
	s.epoch = req.NewEpochID

	return nil
}

type PostBlockRequest struct {
	// Block is a reference of the executed block
	Block *types.Block
}

// PostBlock notifies state sync manager that a block was finalized,
// so that it can build state sync proofs if a block has a commitment submission transaction
func (s *stateSyncManager) PostBlock(req *PostBlockRequest) error {
	commitment, err := getCommitmentMessageSignedTx(req.Block.Transactions)
	if err != nil {
		return err
	}

	// no commitment message -> this is not end of epoch block
	if commitment == nil {
		return nil
	}

	if err := s.state.insertCommitmentMessage(commitment); err != nil {
		return fmt.Errorf("insert commitment message error: %w", err)
	}

	// commitment was submitted, so discard what we have in memory, so we can build a new one
	s.commitment = nil
	if err := s.buildProofs(commitment.Message); err != nil {
		return fmt.Errorf("build commitment proofs error: %w", err)
	}

	return nil
}

// buildProofs builds state sync proofs for the submitted commitment and saves them in boltDb for later execution
func (s *stateSyncManager) buildProofs(commitmentMsg *CommitmentMessage) error {
	s.logger.Debug(
		"[buildProofs] Building proofs for commitment...",
		"fromIndex", commitmentMsg.FromIndex,
		"toIndex", commitmentMsg.ToIndex,
	)

	events, err := s.state.getStateSyncEventsForCommitment(commitmentMsg.FromIndex, commitmentMsg.ToIndex)
	if err != nil {
		return err
	}

	tree, err := createMerkleTree(events)
	if err != nil {
		return err
	}

	stateSyncProofs := make([]*types.StateSyncProof, len(events))

	for i, event := range events {
		p := tree.GenerateProof(uint64(i), 0)

		stateSyncProofs[i] = &types.StateSyncProof{
			Proof:     p,
			StateSync: event,
		}
	}

	s.logger.Debug(
		"[buildProofs] Building proofs for commitment finished.",
		"fromIndex", commitmentMsg.FromIndex,
		"toIndex", commitmentMsg.ToIndex,
	)

	return s.state.insertStateSyncProofs(stateSyncProofs)
}

// buildCommitment builds a new commitment, signs it and gossips its vote for it
func (s *stateSyncManager) buildCommitment(epoch, fromIndex uint64) (*Commitment, error) {
	toIndex := fromIndex + stateSyncCommitmentSize - 1

	// if it is not already built in the previous epoch
	stateSyncEvents, err := s.state.getStateSyncEventsForCommitment(fromIndex, toIndex)
	if err != nil {
		if errors.Is(err, errNotEnoughStateSyncs) {
			s.logger.Debug("[buildCommitment] Not enough state syncs to build a commitment",
				"epoch", epoch, "from state sync index", fromIndex)

			// this is a valid case, there is not enough state syncs
			return nil, nil
		}

		return nil, err
	}

	commitment, err := NewCommitment(epoch, stateSyncEvents)
	if err != nil {
		return nil, err
	}

	hash, err := commitment.Hash()
	if err != nil {
		return nil, err
	}

	hashBytes := hash.Bytes()

	signature, err := s.config.key.Sign(hashBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to sign commitment message. Error: %w", err)
	}

	sig := &MessageSignature{
		From:      s.config.key.String(),
		Signature: signature,
	}

	if _, err = s.state.insertMessageVote(epoch, hashBytes, sig); err != nil {
		return nil, fmt.Errorf(
			"failed to insert signature for hash=%v to the state. Error: %w",
			hex.EncodeToString(hashBytes),
			err,
		)
	}

	// gossip message
	s.Multicast(&TransportMessage{
		Hash:        hashBytes,
		Signature:   signature,
		NodeID:      s.config.key.String(),
		EpochNumber: epoch,
	})

	s.logger.Debug(
		"[buildCommitment] Built commitment",
		"from", commitment.FromIndex,
		"to", commitment.ToIndex,
	)

	return commitment, nil
}

// Multicast publishes given message to the rest of the network
func (s *stateSyncManager) Multicast(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		s.logger.Warn("failed to marshal bridge message", "err", err)

		return
	}

	err = s.config.topic.Publish(&polybftProto.TransportMessage{Data: data})
	if err != nil {
		s.logger.Warn("failed to gossip bridge message", "err", err)
	}
}

// newStateSyncEvent creates an instance of pending state sync event.
func newStateSyncEvent(
	id uint64,
	sender ethgo.Address,
	target ethgo.Address,
	data []byte,
) *types.StateSyncEvent {
	return &types.StateSyncEvent{
		ID:       id,
		Sender:   sender,
		Receiver: target,
		Data:     data,
	}
}

func decodeStateSyncEvent(log *ethgo.Log) (*types.StateSyncEvent, error) {
	raw, err := stateTransferEventABI.ParseLog(log)
	if err != nil {
		return nil, err
	}

	eventGeneric, err := decodeEventData(raw, log,
		func(id *big.Int, sender, receiver ethgo.Address, data []byte) interface{} {
			return newStateSyncEvent(id.Uint64(), sender, receiver, data)
		})
	if err != nil {
		return nil, err
	}

	stateSyncEvent, ok := eventGeneric.(*types.StateSyncEvent)
	if !ok {
		return nil, errors.New("failed to convert event to StateSyncEvent instance")
	}

	return stateSyncEvent, nil
}
