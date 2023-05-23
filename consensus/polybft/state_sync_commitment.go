package polybft

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/merkle-tree"
	"github.com/0xPolygon/polygon-edge/state/runtime/precompiled"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	stTypeBridgeCommitment = "commitment"
	stTypeEndEpoch         = "end-epoch"
)

// PendingCommitment holds merkle trie of bridge transactions accompanied by epoch number
type PendingCommitment struct {
	*contractsapi.StateSyncCommitment
	MerkleTree *merkle.MerkleTree
	Epoch      uint64
}

// NewPendingCommitment creates a new commitment object
func NewPendingCommitment(epoch uint64, stateSyncEvents []*contractsapi.StateSyncedEvent) (*PendingCommitment, error) {
	tree, err := createMerkleTree(stateSyncEvents)
	if err != nil {
		return nil, err
	}

	return &PendingCommitment{
		MerkleTree: tree,
		Epoch:      epoch,
		StateSyncCommitment: &contractsapi.StateSyncCommitment{
			StartID: stateSyncEvents[0].ID,
			EndID:   stateSyncEvents[len(stateSyncEvents)-1].ID,
			Root:    tree.Hash(),
		},
	}, nil
}

// Hash calculates hash value for commitment object.
func (cm *PendingCommitment) Hash() (types.Hash, error) {
	data, err := cm.StateSyncCommitment.EncodeAbi()
	if err != nil {
		return types.Hash{}, err
	}

	return crypto.Keccak256Hash(data), nil
}

var _ contractsapi.StateTransactionInput = &CommitmentMessageSigned{}

// CommitmentMessageSigned encapsulates commitment message with aggregated signatures
type CommitmentMessageSigned struct {
	Message      *contractsapi.StateSyncCommitment
	AggSignature Signature
	PublicKeys   [][]byte
}

// Hash calculates hash value for commitment object.
func (cm *CommitmentMessageSigned) Hash() (types.Hash, error) {
	data, err := cm.Message.EncodeAbi()
	if err != nil {
		return types.Hash{}, err
	}

	return crypto.Keccak256Hash(data), nil
}

// VerifyStateSyncProof validates given state sync proof
// against merkle tree root hash contained in the CommitmentMessage
func (cm *CommitmentMessageSigned) VerifyStateSyncProof(proof []types.Hash,
	stateSync *contractsapi.StateSyncedEvent) error {
	if stateSync == nil {
		return errors.New("no state sync event")
	}

	if stateSync.ID.Uint64() < cm.Message.StartID.Uint64() ||
		stateSync.ID.Uint64() > cm.Message.EndID.Uint64() {
		return errors.New("invalid state sync ID")
	}

	hash, err := stateSync.EncodeAbi()
	if err != nil {
		return err
	}

	return merkle.VerifyProof(stateSync.ID.Uint64()-cm.Message.StartID.Uint64(),
		hash, proof, cm.Message.Root)
}

// ContainsStateSync checks if commitment contains given state sync event
func (cm *CommitmentMessageSigned) ContainsStateSync(stateSyncID uint64) bool {
	return cm.Message.StartID.Uint64() <= stateSyncID && cm.Message.EndID.Uint64() >= stateSyncID
}

// EncodeAbi contains logic for encoding arbitrary data into ABI format
func (cm *CommitmentMessageSigned) EncodeAbi() ([]byte, error) {
	blsVerificationPart, err := precompiled.BlsVerificationABIType.Encode(
		[2]interface{}{cm.PublicKeys, cm.AggSignature.Bitmap})
	if err != nil {
		return nil, err
	}

	commit := &contractsapi.CommitStateReceiverFn{
		Commitment: cm.Message,
		Signature:  cm.AggSignature.AggregatedSignature,
		Bitmap:     blsVerificationPart,
	}

	return commit.EncodeAbi()
}

// DecodeAbi contains logic for decoding given ABI data
func (cm *CommitmentMessageSigned) DecodeAbi(txData []byte) error {
	if len(txData) < abiMethodIDLength {
		return fmt.Errorf("invalid commitment data, len = %d", len(txData))
	}

	commit := contractsapi.CommitStateReceiverFn{}

	err := commit.DecodeAbi(txData)
	if err != nil {
		return err
	}

	decoded, err := precompiled.BlsVerificationABIType.Decode(commit.Bitmap)
	if err != nil {
		return err
	}

	blsMap, isOk := decoded.(map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid commitment data. Bls verification part not in correct format")
	}

	publicKeys, isOk := blsMap["0"].([][]byte)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find public keys part")
	}

	bitmap, isOk := blsMap["1"].([]byte)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find bitmap part")
	}

	*cm = CommitmentMessageSigned{
		Message: commit.Commitment,
		AggSignature: Signature{
			AggregatedSignature: commit.Signature,
			Bitmap:              bitmap,
		},
		PublicKeys: publicKeys,
	}

	return nil
}

// getCommitmentMessageSignedTx returns a CommitmentMessageSigned object from a commit state transaction
func getCommitmentMessageSignedTx(txs []*types.Transaction) (*CommitmentMessageSigned, error) {
	var commitFn contractsapi.CommitStateReceiverFn
	for _, tx := range txs {
		// skip non state CommitmentMessageSigned transactions
		if tx.Type != types.StateTx ||
			len(tx.Input) < abiMethodIDLength ||
			!bytes.Equal(tx.Input[:abiMethodIDLength], commitFn.Sig()) {
			continue
		}

		obj := &CommitmentMessageSigned{}

		if err := obj.DecodeAbi(tx.Input); err != nil {
			return nil, fmt.Errorf("get commitment message signed tx error: %w", err)
		}

		return obj, nil
	}

	return nil, nil
}

// createMerkleTree creates a merkle tree from provided state sync events
// if only one state sync event is provided, a second, empty leaf will be added to merkle tree
// so that we can have a commitment with a single state sync event
func createMerkleTree(stateSyncEvents []*contractsapi.StateSyncedEvent) (*merkle.MerkleTree, error) {
	stateSyncData := make([][]byte, len(stateSyncEvents))

	for i, sse := range stateSyncEvents {
		data, err := sse.EncodeAbi()
		if err != nil {
			return nil, err
		}

		stateSyncData[i] = data
	}

	return merkle.NewMerkleTree(stateSyncData)
}
