package polybft

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	polybftProto "github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/tracker"
	boltdbStore "github.com/umbracle/ethgo/tracker/store/boltdb"
	"google.golang.org/protobuf/proto"
)

type StateSyncManager struct {
	// configuration fields
	logger            hclog.Logger
	state             *State
	stateReceiverAddr types.Address
	jsonrpcAddr       string
	dataDir           string
	topic             topic
	key               *wallet.Key

	// per epoch fields
	lock         sync.Mutex
	commitment   *Commitment
	epoch        uint64
	validatorSet *validatorSet
}

// topic is an interface for a gossip p2p message
type topic interface {
	Publish(obj proto.Message) error
	Subscribe(handler func(obj interface{}, from peer.ID)) error
}

func NewStateSyncManager(logger hclog.Logger, key *wallet.Key, state *State,
	stateReceiverAddr types.Address, jsonrpcAddr string, dataDir string, topic topic) (*StateSyncManager, error) {
	s := &StateSyncManager{
		logger:            logger.Named("state-sync"),
		state:             state,
		stateReceiverAddr: stateReceiverAddr,
		jsonrpcAddr:       jsonrpcAddr,
		dataDir:           dataDir,
		topic:             topic,
		key:               key,
		lock:              sync.Mutex{},
	}

	return s, nil
}

func (s *StateSyncManager) init() error {
	if err := s.initTracker(); err != nil {
		return err
	}

	if err := s.initTransport(); err != nil {
		return err
	}

	return nil
}

func (s *StateSyncManager) initTransport() error {
	return s.topic.Subscribe(func(obj interface{}, _ peer.ID) {
		msg, ok := obj.(*polybftProto.TransportMessage)
		if !ok {
			s.logger.Warn("failed to deliver message, invalid msg", "obj", obj)

			return
		}

		var transportMsg *TransportMessage

		if err := json.Unmarshal(msg.Data, &transportMsg); err != nil {
			s.logger.Warn("failed to deliver message", "error", err)

			return
		}

		if err := s.deliverMessage(transportMsg); err != nil {
			s.logger.Warn("failed to deliver message", "error", err)
		}
	})
}

func (s *StateSyncManager) deliverMessage(msg *TransportMessage) error {
	s.lock.Lock()
	epoch := s.epoch
	valSet := s.validatorSet
	s.lock.Unlock()

	if valSet == nil {
		// Epoch metadata is undefined
		return nil
	}

	if msg.EpochNumber < epoch {
		// or received message for some of the older epochs.
		return nil
	}

	if !valSet.validators.ContainsNodeID(msg.NodeID) {
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

func (s *StateSyncManager) initTracker() error {
	provider, err := jsonrpc.NewClient(s.jsonrpcAddr)
	if err != nil {
		return err
	}

	store, err := boltdbStore.New(filepath.Join(s.dataDir, "/deposit.db"))
	if err != nil {
		return err
	}

	s.logger.Info("Start tracking events", "bridge", s.stateReceiverAddr)

	tt, err := tracker.NewTracker(provider.Eth(),
		tracker.WithBatchSize(10),
		tracker.WithStore(store),
		tracker.WithFilter(&tracker.FilterConfig{
			Async: false,
			Address: []ethgo.Address{
				ethgo.Address(s.stateReceiverAddr),
			},
		}),
	)
	if err != nil {
		return err
	}

	go func() {
		go func() {
			if err := tt.Sync(context.Background()); err != nil {
				s.logger.Error("Event tracker", "failed to sync", err)
			}
		}()

		go func() {
			for {
				select {
				case evnt := <-tt.EventCh:
					if len(evnt.Removed) != 0 {
						panic("this will not happen anymore after tracker v2")
					}

					for _, log := range evnt.Added {
						fmt.Println("- log -")
						fmt.Println(log)

						if stateTransferEventABI.Match(log) {
							if err := s.addLog(log); err != nil {
								s.logger.Error("failed to decode state sync event", "hash", log.TransactionHash, "error", err)
							}
						}
					}
				case <-tt.DoneCh:
					s.logger.Info("Historical sync done")
				}
			}
		}()
	}()

	return nil
}

func (s *StateSyncManager) addLog(eventLog *ethgo.Log) error {
	s.logger.Info(
		"Add State sync event",
		"block", eventLog.BlockNumber,
		"hash", eventLog.TransactionHash,
		"index", eventLog.LogIndex,
	)

	event, err := decodeStateSyncEvent(eventLog)
	if err != nil {
		return err
	}

	if err := s.state.insertStateSyncEvent(event); err != nil {
		return err
	}

	return nil
}

func (s *StateSyncManager) Commitment() (*CommitmentMessageSigned, error) {
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
		return nil, err
	}

	msg := &CommitmentMessageSigned{
		Message:      commitmentMessage,
		AggSignature: aggregatedSignature,
		PublicKeys:   publicKeys,
	}

	return msg, nil
}

func (s *StateSyncManager) getAggSignatureForCommitmentMessage(commitment *Commitment) (Signature, [][]byte, error) {
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

	// Epoch is the new epoch
	Epoch uint64

	// SystemState is the state of the governance smart contracts
	// after this block
	SystemState SystemState

	// ValidatorSet is the validator set for this epoch
	ValidatorSet *validatorSet
}

func (s *StateSyncManager) PostEpoch(req *PostEpochRequest) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// build a new commitment at the end of the epoch
	nextCommittedIndex, err := req.SystemState.GetNextCommittedIndex()
	if err != nil {
		return err
	}

	commitment, err := s.buildCommitment(req.Epoch+1, nextCommittedIndex)
	if err != nil {
		return err
	}

	s.commitment = commitment
	s.validatorSet = req.ValidatorSet
	s.epoch = req.Epoch

	return nil
}

type PostBlockRequest struct {
	// Block is a reference of the executed block
	Block *types.Block
}

func (s *StateSyncManager) PostBlock(req *PostBlockRequest) error {
	// handle commitment and proofs creation
	if err := s.getCommitmentFromTransactions(req.Block.Transactions); err != nil {
		return err
	}

	return nil
}

func (s *StateSyncManager) getCommitmentFromTransactions(txs []*types.Transaction) error {
	commitment, err := getCommitmentMessageSignedTx(txs)
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

	if s.commitment != nil {
		// TODO: We have to build the proofs for a commitment that arrives
		if err := s.buildProofs(s.commitment, commitment.Message); err != nil {
			return fmt.Errorf("build commitment proofs error: %w", err)
		}
	}

	return nil
}

func (s *StateSyncManager) buildProofs(commitment *Commitment, commitmentMsg *CommitmentMessage) error {
	s.logger.Debug(
		"[buildProofs] Building proofs...",
		"fromIndex", commitmentMsg.FromIndex,
		"toIndex", commitmentMsg.ToIndex,
	)

	events, err := s.state.getStateSyncEventsForCommitment(commitmentMsg.FromIndex, commitmentMsg.ToIndex)
	if err != nil {
		return err
	}

	stateSyncProofs := make([]*types.StateSyncProof, len(events))

	for i, event := range events {
		p := commitment.MerkleTree.GenerateProof(uint64(i), 0)

		stateSyncProofs[i] = &types.StateSyncProof{
			Proof:     p,
			StateSync: event,
		}
	}

	s.logger.Debug(
		"[buildProofs] Building proofs finished.",
		"fromIndex", commitmentMsg.FromIndex,
		"toIndex", commitmentMsg.ToIndex,
	)

	return s.state.insertStateSyncProofs(stateSyncProofs)
}

func (s *StateSyncManager) buildCommitment(epoch, fromIndex uint64) (*Commitment, error) {
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

	signature, err := s.key.Sign(hashBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to sign commitment message. Error: %w", err)
	}

	sig := &MessageSignature{
		From:      s.key.String(),
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
		NodeID:      s.key.String(),
		EpochNumber: epoch,
	})

	s.logger.Debug(
		"[buildCommitment] Built commitment",
		"from", commitment.FromIndex,
		"to", commitment.ToIndex,
	)

	return commitment, nil
}

func (s *StateSyncManager) Multicast(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		s.logger.Warn("failed to marshal bridge message", "err", err)

		return
	}

	err = s.topic.Publish(&polybftProto.TransportMessage{Data: data})
	if err != nil {
		s.logger.Warn("failed to gossip bridge message", "err", err)
	}
}
