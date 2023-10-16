package polybft

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/umbracle/ethgo"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	polybftProto "github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/tracker"
	"github.com/0xPolygon/polygon-edge/types"
)

type Runtime interface {
	IsActiveValidator() bool
}

type StateSyncProof struct {
	Proof     []types.Hash
	StateSync *contractsapi.StateSyncedEvent
}

// StateSyncManager is an interface that defines functions for state sync workflow
type StateSyncManager interface {
	EventSubscriber
	Init() error
	Close()
	Commitment(blockNumber uint64) (*CommitmentMessageSigned, error)
	GetStateSyncProof(stateSyncID uint64) (types.Proof, error)
	PostBlock(req *PostBlockRequest) error
	PostEpoch(req *PostEpochRequest) error
}

var _ StateSyncManager = (*dummyStateSyncManager)(nil)

// dummyStateSyncManager is used when bridge is not enabled
type dummyStateSyncManager struct{}

func (d *dummyStateSyncManager) Init() error { return nil }
func (d *dummyStateSyncManager) Close()      {}
func (d *dummyStateSyncManager) Commitment(blockNumber uint64) (*CommitmentMessageSigned, error) {
	return nil, nil
}
func (d *dummyStateSyncManager) PostBlock(req *PostBlockRequest) error { return nil }
func (d *dummyStateSyncManager) PostEpoch(req *PostEpochRequest) error { return nil }
func (d *dummyStateSyncManager) GetStateSyncProof(stateSyncID uint64) (types.Proof, error) {
	return types.Proof{}, nil
}

// EventSubscriber implementation
func (d *dummyStateSyncManager) GetLogFilters() map[types.Address][]types.Hash {
	return make(map[types.Address][]types.Hash)
}
func (d *dummyStateSyncManager) ProcessLog(header *types.Header,
	log *ethgo.Log, dbTx *bolt.Tx) error {
	return nil
}

// stateSyncConfig holds the configuration data of state sync manager
type stateSyncConfig struct {
	stateSenderAddr          types.Address
	stateSenderStartBlock    uint64
	jsonrpcAddr              string
	dataDir                  string
	topic                    topic
	key                      *wallet.Key
	maxCommitmentSize        uint64
	numBlockConfirmations    uint64
	blockTrackerPollInterval time.Duration
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
	validatorSet       validator.ValidatorSet
	epoch              uint64
	nextCommittedIndex uint64

	runtime Runtime
}

// topic is an interface for p2p message gossiping
type topic interface {
	Publish(obj proto.Message) error
	Subscribe(handler func(obj interface{}, from peer.ID)) error
}

// newStateSyncManager creates a new instance of state sync manager
func newStateSyncManager(logger hclog.Logger, state *State, config *stateSyncConfig,
	runtime Runtime) *stateSyncManager {
	return &stateSyncManager{
		logger:  logger,
		state:   state,
		config:  config,
		closeCh: make(chan struct{}),
		runtime: runtime,
	}
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
		s.logger,
		s.config.blockTrackerPollInterval)

	go func() {
		<-s.closeCh
		cancelFn()
	}()

	return evtTracker.Start(ctx)
}

// initTransport subscribes to bridge topics (getting votes for commitments)
func (s *stateSyncManager) initTransport() error {
	return s.config.topic.Subscribe(func(obj interface{}, _ peer.ID) {
		if !s.runtime.IsActiveValidator() {
			// don't save votes if not a validator
			return
		}

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

	numSignatures, err := s.state.StateSyncStore.insertMessageVote(msg.EpochNumber, msg.Hash, msgVote, nil)
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
func (s *stateSyncManager) verifyVoteSignature(valSet validator.ValidatorSet, signerAddr types.Address,
	signature []byte, hash []byte) error {
	validator := valSet.Accounts().GetValidatorMetadata(signerAddr)
	if validator == nil {
		return fmt.Errorf("unable to resolve validator %s", signerAddr)
	}

	unmarshaledSignature, err := bls.UnmarshalSignature(signature)
	if err != nil {
		return fmt.Errorf("failed to unmarshal signature from signer %s, %w", signerAddr.String(), err)
	}

	if !unmarshaledSignature.Verify(validator.BlsKey, hash, signer.DomainStateReceiver) {
		return fmt.Errorf("incorrect signature from %s", signerAddr)
	}

	return nil
}

// AddLog saves the received log from event tracker if it matches a state sync event ABI
func (s *stateSyncManager) AddLog(eventLog *ethgo.Log) error {
	event := &contractsapi.StateSyncedEvent{}

	doesMatch, err := event.ParseLog(eventLog)
	if !doesMatch {
		return nil
	}

	s.logger.Info(
		"Add State sync event",
		"block", eventLog.BlockNumber,
		"hash", eventLog.TransactionHash,
		"index", eventLog.LogIndex,
	)

	if err != nil {
		s.logger.Error("could not decode state sync event", "err", err)

		return err
	}

	if err := s.state.StateSyncStore.insertStateSyncEvent(event); err != nil {
		s.logger.Error("could not save state sync event to boltDb", "err", err)

		return err
	}

	if err := s.buildCommitment(nil); err != nil {
		// we don't return an error here. If state sync event is inserted in db,
		// we will just try to build a commitment on next block or next event arrival
		s.logger.Error("could not build a commitment on arrival of new state sync", "err", err, "stateSyncID", event.ID)
	}

	return nil
}

// Commitment returns a commitment to be submitted if there is a pending commitment with quorum
func (s *stateSyncManager) Commitment(blockNumber uint64) (*CommitmentMessageSigned, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var largestCommitment *CommitmentMessageSigned

	// we start from the end, since last pending commitment is the largest one
	for i := len(s.pendingCommitments) - 1; i >= 0; i-- {
		commitment := s.pendingCommitments[i]
		aggregatedSignature, publicKeys, err := s.getAggSignatureForCommitmentMessage(blockNumber, commitment)

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
func (s *stateSyncManager) getAggSignatureForCommitmentMessage(blockNumber uint64,
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

	if !validatorSet.HasQuorum(blockNumber, signers) {
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

	return s.buildCommitment(req.DBTx)
}

// PostBlock notifies state sync manager that a block was finalized,
// so that it can build state sync proofs if a block has a commitment submission transaction.
// Additionally, it will remove any processed state sync events and their proofs from the store.
func (s *stateSyncManager) PostBlock(req *PostBlockRequest) error {
	commitment, err := getCommitmentMessageSignedTx(req.FullBlock.Block.Transactions)
	if err != nil {
		return err
	}

	// no commitment message -> this is not end of sprint block
	if commitment == nil {
		return nil
	}

	if err := s.state.StateSyncStore.insertCommitmentMessage(commitment, req.DBTx); err != nil {
		return fmt.Errorf("insert commitment message error: %w", err)
	}

	if err := s.buildProofs(commitment.Message, req.DBTx); err != nil {
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
		// if we are all synced up, commitment will be saved through PostBlock, but we won't have proofs,
		// so we will build them now and save them to db so that we have proofs for missed commitment
		commitment, err := s.state.StateSyncStore.getCommitmentForStateSync(stateSyncID)
		if err != nil {
			return types.Proof{}, fmt.Errorf("cannot find commitment for StateSync id %d: %w", stateSyncID, err)
		}

		if err := s.buildProofs(commitment.Message, nil); err != nil {
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
func (s *stateSyncManager) buildProofs(commitmentMsg *contractsapi.StateSyncCommitment,
	dbTx *bolt.Tx) error {
	from := commitmentMsg.StartID.Uint64()
	to := commitmentMsg.EndID.Uint64()

	s.logger.Debug(
		"[buildProofs] Building proofs for commitment...",
		"fromIndex", from,
		"toIndex", to,
	)

	events, err := s.state.StateSyncStore.getStateSyncEventsForCommitment(from, to, dbTx)
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

	return s.state.StateSyncStore.insertStateSyncProofs(stateSyncProofs, dbTx)
}

// buildCommitment builds a new commitment, signs it and gossips its vote for it
func (s *stateSyncManager) buildCommitment(dbTx *bolt.Tx) error {
	if !s.runtime.IsActiveValidator() {
		// don't build commitment if not a validator
		return nil
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	stateSyncEvents, err := s.state.StateSyncStore.getStateSyncEventsForCommitment(s.nextCommittedIndex,
		s.nextCommittedIndex+s.config.maxCommitmentSize-1, dbTx)
	if err != nil && !errors.Is(err, errNotEnoughStateSyncs) {
		return fmt.Errorf("failed to get state sync events for commitment. Error: %w", err)
	}

	if len(stateSyncEvents) == 0 {
		// there are no state sync events
		return nil
	}

	if len(s.pendingCommitments) > 0 &&
		s.pendingCommitments[len(s.pendingCommitments)-1].StartID.Cmp(stateSyncEvents[len(stateSyncEvents)-1].ID) >= 0 {
		// already built a commitment of this size which is pending to be submitted
		return nil
	}

	commitment, err := NewPendingCommitment(s.epoch, stateSyncEvents)
	if err != nil {
		return err
	}

	hash, err := commitment.Hash()
	if err != nil {
		return fmt.Errorf("failed to generate hash for commitment. Error: %w", err)
	}

	hashBytes := hash.Bytes()

	signature, err := s.config.key.SignWithDomain(hashBytes, signer.DomainStateReceiver)
	if err != nil {
		return fmt.Errorf("failed to sign commitment message. Error: %w", err)
	}

	sig := &MessageSignature{
		From:      s.config.key.String(),
		Signature: signature,
	}

	if _, err = s.state.StateSyncStore.insertMessageVote(s.epoch, hashBytes, sig, dbTx); err != nil {
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
		EpochNumber: s.epoch,
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

// EventSubscriber implementation

// GetLogFilters returns a map of log filters for getting desired events,
// where the key is the address of contract that emits desired events,
// and the value is a slice of signatures of events we want to get.
// This function is the implementation of EventSubscriber interface
func (s *stateSyncManager) GetLogFilters() map[types.Address][]types.Hash {
	var stateSyncResultEvent contractsapi.StateSyncResultEvent

	return map[types.Address][]types.Hash{
		contracts.StateReceiverContract: {types.Hash(stateSyncResultEvent.Sig())},
	}
}

// ProcessLog is the implementation of EventSubscriber interface,
// used to handle a log defined in GetLogFilters, provided by event provider
func (s *stateSyncManager) ProcessLog(header *types.Header, log *ethgo.Log, dbTx *bolt.Tx) error {
	var stateSyncResultEvent contractsapi.StateSyncResultEvent

	doesMatch, err := stateSyncResultEvent.ParseLog(log)
	if err != nil {
		return err
	}

	if !doesMatch {
		return nil
	}

	return s.state.StateSyncStore.removeStateSyncEventsAndProofs([]uint64{stateSyncResultEvent.Counter.Uint64()})
}
