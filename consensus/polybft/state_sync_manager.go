package polybft

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sync"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
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
	// minimum number of stateSyncEvents that a commitment can have
	// (minimum number is 2 because smart contract expects that the merkle tree has at least two leaves)
	minCommitmentSize = 2
)

type StateSyncProof struct {
	Proof     []types.Hash
	StateSync *contractsapi.StateSyncedEvent
}

// StateSyncManager is an interface that defines functions for state sync workflow
type StateSyncManager interface {
	Init() error
	Close()
	Commitment() (*CommitmentMessageSigned, error)
	GetStateSyncProof(stateSyncID uint64) (types.Proof, error)
	PostBlock(req *PostBlockRequest) error
	PostEpoch(req *PostEpochRequest) error
}

var _ StateSyncManager = (*dummyStateSyncManager)(nil)

// dummyStateSyncManager is used when bridge is not enabled
type dummyStateSyncManager struct{}

func (n *dummyStateSyncManager) Init() error                                   { return nil }
func (n *dummyStateSyncManager) Close()                                        {}
func (n *dummyStateSyncManager) Commitment() (*CommitmentMessageSigned, error) { return nil, nil }
func (n *dummyStateSyncManager) PostBlock(req *PostBlockRequest) error         { return nil }
func (n *dummyStateSyncManager) PostEpoch(req *PostEpochRequest) error         { return nil }
func (n *dummyStateSyncManager) GetStateSyncProof(stateSyncID uint64) (types.Proof, error) {
	return types.Proof{}, nil
}

// stateSyncConfig holds the configuration data of state sync manager
type stateSyncConfig struct {
	stateSenderAddr       types.Address
	stateSenderStartBlock uint64
	jsonrpcAddr           string
	dataDir               string
	topic                 topic
	key                   *wallet.Key
	maxCommitmentSize     uint64
	numBlockConfirmations uint64
}

var _ StateSyncManager = (*stateSyncManager)(nil)

// stateSyncManager is a struct that manages the workflow of
// saving and querying state sync events, and creating, and submitting new commitments
type stateSyncManager struct {
	logger hclog.Logger
	state  *State

	config  *stateSyncConfig
	closeCh chan struct{}

	// per epoch fields
	lock               sync.RWMutex
	pendingCommitments []*PendingCommitment
	validatorSet       ValidatorSet
	epoch              uint64
	nextCommittedIndex uint64
}

// topic is an interface for p2p message gossiping
type topic interface {
	Publish(obj proto.Message) error
	Subscribe(handler func(obj interface{}, from peer.ID)) error
}

// newStateSyncManager creates a new instance of state sync manager
func newStateSyncManager(logger hclog.Logger, state *State, config *stateSyncConfig) (*stateSyncManager, error) {
	s := &stateSyncManager{
		logger:  logger,
		state:   state,
		config:  config,
		closeCh: make(chan struct{}),
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

func (s *stateSyncManager) Close() {
	close(s.closeCh)
}

// initTracker starts a new event tracker (to receive new state sync events)
func (s *stateSyncManager) initTracker() error {
	ctx, cancelFn := context.WithCancel(context.Background())

	evtTracker := tracker.NewEventTracker(
		path.Join(s.config.dataDir, "/deposit.db"),
		s.config.jsonrpcAddr,
		ethgo.Address(s.config.stateSenderAddr),
		s,
		s.config.numBlockConfirmations,
		s.config.stateSenderStartBlock,
		s.logger)

	go func() {
		<-s.closeCh
		cancelFn()
	}()

	return evtTracker.Start(ctx)
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
	s.lock.RLock()
	epoch := s.epoch
	valSet := s.validatorSet
	s.lock.RUnlock()

	if valSet == nil || msg.EpochNumber != epoch {
		// Epoch metadata is undefined or received a message for the irrelevant epoch
		return nil
	}

	if err := s.verifyVoteSignature(valSet, types.StringToAddress(msg.From), msg.Signature, msg.Hash); err != nil {
		return fmt.Errorf("error verifying vote signature: %w", err)
	}

	msgVote := &MessageSignature{
		From:      msg.From,
		Signature: msg.Signature,
	}

	numSignatures, err := s.state.StateSyncStore.insertMessageVote(msg.EpochNumber, msg.Hash, msgVote)
	if err != nil {
		return fmt.Errorf("error inserting message vote: %w", err)
	}

	s.logger.Info(
		"deliver message",
		"hash", hex.EncodeToString(msg.Hash),
		"sender", msg.From,
		"signatures", numSignatures,
	)

	return nil
}

// Verifies signature of the message against the public key of the signer and checks if the signer is a validator
func (s *stateSyncManager) verifyVoteSignature(valSet ValidatorSet, signer types.Address, signature []byte,
	hash []byte) error {
	validator := valSet.Accounts().GetValidatorMetadata(signer)
	if validator == nil {
		return fmt.Errorf("unable to resolve validator %s", signer)
	}

	unmarshaledSignature, err := bls.UnmarshalSignature(signature)
	if err != nil {
		return fmt.Errorf("failed to unmarshal signature from signer %s, %w", signer.String(), err)
	}

	if !unmarshaledSignature.Verify(validator.BlsKey, hash, bls.DomainStateReceiver) {
		return fmt.Errorf("incorrect signature from %s", signer)
	}

	return nil
}

// AddLog saves the received log from event tracker if it matches a state sync event ABI
func (s *stateSyncManager) AddLog(eventLog *ethgo.Log) {
	event := &contractsapi.StateSyncedEvent{}

	doesMatch, err := event.ParseLog(eventLog)
	if !doesMatch {
		return
	}

	s.logger.Info(
		"Add State sync event",
		"block", eventLog.BlockNumber,
		"hash", eventLog.TransactionHash,
		"index", eventLog.LogIndex,
	)

	if err != nil {
		s.logger.Error("could not decode state sync event", "err", err)

		return
	}

	if err := s.state.StateSyncStore.insertStateSyncEvent(event); err != nil {
		s.logger.Error("could not save state sync event to boltDb", "err", err)

		return
	}

	if err := s.buildCommitment(); err != nil {
		s.logger.Error("could not build a commitment on arrival of new state sync", "err", err, "stateSyncID", event.ID)
	}
}

// Commitment returns a commitment to be submitted if there is a pending commitment with quorum
func (s *stateSyncManager) Commitment() (*CommitmentMessageSigned, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var largestCommitment *CommitmentMessageSigned

	// we start from the end, since last pending commitment is the largest one
	for i := len(s.pendingCommitments) - 1; i >= 0; i-- {
		commitment := s.pendingCommitments[i]
		aggregatedSignature, publicKeys, err := s.getAggSignatureForCommitmentMessage(commitment)

		if err != nil {
			if errors.Is(err, errQuorumNotReached) {
				// a valid case, commitment has no quorum, we should not return an error
				s.logger.Debug("can not submit a commitment, quorum not reached",
					"from", commitment.StartID.Uint64(),
					"to", commitment.EndID.Uint64())

				continue
			}

			return nil, err
		}

		largestCommitment = &CommitmentMessageSigned{
			Message:      commitment.StateSyncCommitment,
			AggSignature: aggregatedSignature,
			PublicKeys:   publicKeys,
		}

		break
	}

	return largestCommitment, nil
}

// getAggSignatureForCommitmentMessage checks if pending commitment has quorum,
// and if it does, aggregates the signatures
func (s *stateSyncManager) getAggSignatureForCommitmentMessage(
	commitment *PendingCommitment) (Signature, [][]byte, error) {
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
	votes, err := s.state.StateSyncStore.getMessageVotes(commitment.Epoch, commitmentHash.Bytes())
	if err != nil {
		return Signature{}, nil, err
	}

	var signatures bls.Signatures

	publicKeys := make([][]byte, 0)
	bmap := bitmap.Bitmap{}
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

		bmap.Set(uint64(index))

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
		Bitmap:              bmap,
	}

	return result, publicKeys, nil
}

// PostEpoch notifies the state sync manager that an epoch has changed,
// so that it can discard any previous epoch commitments, and build a new one (since validator set changed)
func (s *stateSyncManager) PostEpoch(req *PostEpochRequest) error {
	s.lock.Lock()

	s.pendingCommitments = nil
	s.validatorSet = req.ValidatorSet
	s.epoch = req.NewEpochID

	// build a new commitment at the end of the epoch
	nextCommittedIndex, err := req.SystemState.GetNextCommittedIndex()
	if err != nil {
		s.lock.Unlock()

		return err
	}

	s.nextCommittedIndex = nextCommittedIndex

	s.lock.Unlock()

	return s.buildCommitment()
}

// PostBlock notifies state sync manager that a block was finalized,
// so that it can build state sync proofs if a block has a commitment submission transaction
func (s *stateSyncManager) PostBlock(req *PostBlockRequest) error {
	commitment, err := getCommitmentMessageSignedTx(req.FullBlock.Block.Transactions)
	if err != nil {
		return err
	}

	// no commitment message -> this is not end of epoch block
	if commitment == nil {
		return nil
	}

	if err := s.state.StateSyncStore.insertCommitmentMessage(commitment); err != nil {
		return fmt.Errorf("insert commitment message error: %w", err)
	}

	if err := s.buildProofs(commitment.Message); err != nil {
		return fmt.Errorf("build commitment proofs error: %w", err)
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	// update the nextCommittedIndex since a commitment was submitted
	s.nextCommittedIndex = commitment.Message.EndID.Uint64() + 1
	// commitment was submitted, so discard what we have in memory, so we can build a new one
	s.pendingCommitments = nil

	return nil
}

// GetStateSyncProof returns the proof for the state sync
func (s *stateSyncManager) GetStateSyncProof(stateSyncID uint64) (types.Proof, error) {
	stateSyncProof, err := s.state.StateSyncStore.getStateSyncProof(stateSyncID)
	if err != nil {
		return types.Proof{}, fmt.Errorf("cannot get state sync proof for StateSync id %d: %w", stateSyncID, err)
	}

	if stateSyncProof == nil {
		// check if we might've missed a commitment. if it is so, we didn't build proofs for it while syncing
		// if we are all synced up, commitment will be saved through PostBlock, but we wont have proofs,
		// so we will build them now and save them to db so that we have proofs for missed commitment
		commitment, err := s.state.StateSyncStore.getCommitmentForStateSync(stateSyncID)
		if err != nil {
			return types.Proof{}, fmt.Errorf("cannot find commitment for StateSync id %d: %w", stateSyncID, err)
		}

		if err := s.buildProofs(commitment.Message); err != nil {
			return types.Proof{}, fmt.Errorf("cannot build proofs for commitment for StateSync id %d: %w", stateSyncID, err)
		}

		stateSyncProof, err = s.state.StateSyncStore.getStateSyncProof(stateSyncID)
		if err != nil {
			return types.Proof{}, fmt.Errorf("cannot get state sync proof for StateSync id %d: %w", stateSyncID, err)
		}
	}

	return types.Proof{
		Data: stateSyncProof.Proof,
		Metadata: map[string]interface{}{
			"StateSync": stateSyncProof.StateSync,
		},
	}, nil
}

// buildProofs builds state sync proofs for the submitted commitment and saves them in boltDb for later execution
func (s *stateSyncManager) buildProofs(commitmentMsg *contractsapi.StateSyncCommitment) error {
	from := commitmentMsg.StartID.Uint64()
	to := commitmentMsg.EndID.Uint64()

	s.logger.Debug(
		"[buildProofs] Building proofs for commitment...",
		"fromIndex", from,
		"toIndex", to,
	)

	events, err := s.state.StateSyncStore.getStateSyncEventsForCommitment(from, to)
	if err != nil {
		return fmt.Errorf("failed to get state sync events for commitment to build proofs. Error: %w", err)
	}

	tree, err := createMerkleTree(events)
	if err != nil {
		return fmt.Errorf("could not create merkle tree. error: %w", err)
	}

	stateSyncProofs := make([]*StateSyncProof, len(events))

	for i, event := range events {
		leaf, err := event.EncodeAbi()
		if err != nil {
			return fmt.Errorf("could not encode state sync event. error: %w", err)
		}

		p, err := tree.GenerateProof(leaf)
		if err != nil {
			return fmt.Errorf("error generating proof for event: %v. error: %w", event.ID, err)
		}

		stateSyncProofs[i] = &StateSyncProof{
			Proof:     p,
			StateSync: event,
		}
	}

	s.logger.Debug(
		"[buildProofs] Building proofs for commitment finished.",
		"fromIndex", from,
		"toIndex", to,
	)

	return s.state.StateSyncStore.insertStateSyncProofs(stateSyncProofs)
}

// buildCommitment builds a new commitment, signs it and gossips its vote for it
func (s *stateSyncManager) buildCommitment() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	epoch := s.epoch
	fromIndex := s.nextCommittedIndex

	stateSyncEvents, err := s.state.StateSyncStore.getStateSyncEventsForCommitment(fromIndex,
		fromIndex+s.config.maxCommitmentSize-1)
	if err != nil && !errors.Is(err, errNotEnoughStateSyncs) {
		return fmt.Errorf("failed to get state sync events for commitment. Error: %w", err)
	}

	if len(stateSyncEvents) < minCommitmentSize {
		// there is not enough state sync events to build at least the minimum commitment
		return nil
	}

	if len(s.pendingCommitments) > 0 &&
		s.pendingCommitments[len(s.pendingCommitments)-1].StartID.Cmp(stateSyncEvents[len(stateSyncEvents)-1].ID) >= 0 {
		// already built a commitment of this size which is pending to be submitted
		return nil
	}

	commitment, err := NewPendingCommitment(epoch, stateSyncEvents)
	if err != nil {
		return err
	}

	hash, err := commitment.Hash()
	if err != nil {
		return fmt.Errorf("failed to generate hash for commitment. Error: %w", err)
	}

	hashBytes := hash.Bytes()

	signature, err := s.config.key.SignWithDomain(hashBytes, bls.DomainStateReceiver)
	if err != nil {
		return fmt.Errorf("failed to sign commitment message. Error: %w", err)
	}

	sig := &MessageSignature{
		From:      s.config.key.String(),
		Signature: signature,
	}

	if _, err = s.state.StateSyncStore.insertMessageVote(epoch, hashBytes, sig); err != nil {
		return fmt.Errorf(
			"failed to insert signature for hash=%v to the state. Error: %w",
			hex.EncodeToString(hashBytes),
			err,
		)
	}

	// gossip message
	s.multicast(&TransportMessage{
		Hash:        hashBytes,
		Signature:   signature,
		From:        s.config.key.String(),
		EpochNumber: epoch,
	})

	s.logger.Debug(
		"[buildCommitment] Built commitment",
		"from", commitment.StartID.Uint64(),
		"to", commitment.EndID.Uint64(),
	)

	s.pendingCommitments = append(s.pendingCommitments, commitment)

	return nil
}

// multicast publishes given message to the rest of the network
func (s *stateSyncManager) multicast(msg interface{}) {
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
