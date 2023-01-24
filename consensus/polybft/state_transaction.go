package polybft

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	gensc "github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state/runtime/precompiled"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/mitchellh/mapstructure"
	"github.com/umbracle/ethgo/abi"
)

// StateTransactionInput is an abstraction for different state transaction inputs
type StateTransactionInput interface {
	// EncodeAbi contains logic for encoding arbitrary data into ABI format
	EncodeAbi() ([]byte, error)
	// DecodeAbi contains logic for decoding given ABI data
	DecodeAbi(b []byte) error
	// Type returns type of state transaction input
	Type() StateTransactionType
}

type StateTransactionType string

const (
	abiMethodIDLength      = 4
	stTypeBridgeCommitment = "commitment"
	stTypeEndEpoch         = "end-epoch"
)

// PendingCommitment holds merkle trie of bridge transactions accompanied by epoch number
type PendingCommitment struct {
	*gensc.Commitment
	MerkleTree *MerkleTree
	Epoch      uint64
}

// NewCommitment creates a new commitment object
func NewCommitment(epoch uint64, stateSyncEvents []*types.StateSyncEvent) (*PendingCommitment, error) {
	tree, err := createMerkleTree(stateSyncEvents)
	if err != nil {
		return nil, err
	}

	return &PendingCommitment{
		MerkleTree: tree,
		Epoch:      epoch,
		Commitment: &gensc.Commitment{
			StartID: big.NewInt(int64(stateSyncEvents[0].ID)),
			EndID:   big.NewInt(int64(stateSyncEvents[len(stateSyncEvents)-1].ID)),
			Root:    tree.Hash(),
		},
	}, nil
}

// Hash calculates hash value for commitment object.
func (cm *PendingCommitment) Hash() (types.Hash, error) {
	data, err := cm.Commitment.EncodeAbi()
	if err != nil {
		return types.Hash{}, err
	}

	return crypto.Keccak256Hash(data), nil
}

var _ StateTransactionInput = &CommitmentMessageSigned{}

// CommitmentMessageSigned encapsulates commitment message with aggregated signatures
type CommitmentMessageSigned struct {
	Message      *gensc.Commitment
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
// against merkle trie root hash contained in the CommitmentMessage
func (cm *CommitmentMessageSigned) VerifyStateSyncProof(stateSyncProof *types.StateSyncProof) error {
	if stateSyncProof.StateSync == nil {
		return errors.New("no state sync event")
	}

	hash, err := stateSyncProof.StateSync.EncodeAbi()
	if err != nil {
		return err
	}

	return VerifyProof(stateSyncProof.StateSync.ID-cm.Message.StartID.Uint64(),
		hash, stateSyncProof.Proof, cm.Message.Root)
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

	gensc.StateReceiverContract.Commit.Commitment = cm.Message
	gensc.StateReceiverContract.Commit.Signature = cm.AggSignature.AggregatedSignature
	gensc.StateReceiverContract.Commit.Bitmap = blsVerificationPart

	return gensc.StateReceiverContract.Commit.EncodeAbi()
}

// DecodeAbi contains logic for decoding given ABI data
func (cm *CommitmentMessageSigned) DecodeAbi(txData []byte) error {
	if len(txData) < abiMethodIDLength {
		return fmt.Errorf("invalid commitment data, len = %d", len(txData))
	}

	err := gensc.StateReceiverContract.Commit.DecodeAbi(txData)
	if err != nil {
		return err
	}

	decoded, err := precompiled.BlsVerificationABIType.Decode(gensc.StateReceiverContract.Commit.Bitmap)
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
		Message: gensc.StateReceiverContract.Commit.Commitment,
		AggSignature: Signature{
			AggregatedSignature: gensc.StateReceiverContract.Commit.Signature,
			Bitmap:              bitmap,
		},
		PublicKeys: publicKeys,
	}

	return nil
}

// Type returns type of state transaction input
func (cm *CommitmentMessageSigned) Type() StateTransactionType {
	return stTypeBridgeCommitment
}

func decodeStateTransaction(txData []byte) (StateTransactionInput, error) {
	if len(txData) < abiMethodIDLength {
		return nil, fmt.Errorf("state transactions have input")
	}

	sig := txData[:abiMethodIDLength]

	var obj StateTransactionInput

	if bytes.Equal(sig, gensc.StateReceiver.Abi.Methods["commit"].ID()) {
		// bridge commitment
		obj = &CommitmentMessageSigned{}
	} else {
		return nil, fmt.Errorf("unknown state transaction")
	}

	if err := obj.DecodeAbi(txData); err != nil {
		return nil, err
	}

	return obj, nil
}

func getCommitmentMessageSignedTx(txs []*types.Transaction) (*CommitmentMessageSigned, error) {
	for _, tx := range txs {
		// skip non state CommitmentMessageSigned transactions
		if tx.Type != types.StateTx ||
			len(tx.Input) < abiMethodIDLength ||
			!bytes.Equal(tx.Input[:abiMethodIDLength], gensc.StateReceiver.Abi.Methods["commit"].ID()) {
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

func createMerkleTree(stateSyncEvents []*types.StateSyncEvent) (*MerkleTree, error) {
	ssh := make([][]byte, len(stateSyncEvents))

	for i, sse := range stateSyncEvents {
		data, err := sse.EncodeAbi()
		if err != nil {
			return nil, err
		}

		ssh[i] = data
	}

	return NewMerkleTree(ssh)
}

var _ StateTransactionInput = &CommitEpoch{}

var (
	commitEpochMethod, _ = abi.NewMethod("function commitEpoch(" +
		// new epoch id
		"uint256 id," +
		// Epoch
		"tuple(uint256 startBlock, uint256 endBlock, bytes32 epochRoot) epoch," +
		// Uptime
		"tuple(uint256 epochId,tuple(address validator,uint256 signedBlocks)[] uptimeData,uint256 totalBlocks) uptime)")
)

// Epoch holds the data about epoch execution (when it started and when it ended)
type Epoch struct {
	StartBlock uint64     `abi:"startBlock"`
	EndBlock   uint64     `abi:"endBlock"`
	EpochRoot  types.Hash `abi:"epochRoot"`
}

// Uptime holds the data about number of times validators sealed blocks
// in a given epoch
type Uptime struct {
	EpochID     uint64            `abi:"epochId"`
	UptimeData  []ValidatorUptime `abi:"uptimeData"`
	TotalBlocks uint64            `abi:"totalBlocks"`
}

func (u *Uptime) addValidatorUptime(address types.Address, count uint64) {
	if u.UptimeData == nil {
		u.UptimeData = []ValidatorUptime{}
	}

	u.UptimeData = append(u.UptimeData, ValidatorUptime{
		Address: address,
		Count:   count,
	})
}

// ValidatorUptime contains data about how many blocks a given validator has sealed
// in a single period (epoch)
type ValidatorUptime struct {
	Address types.Address `abi:"validator"`
	Count   uint64        `abi:"signedBlocks"`
}

// CommitEpoch contains data that is sent to ChildValidatorSet contract
// to distribute rewards on the end of an epoch
type CommitEpoch struct {
	EpochID uint64 `abi:"id"`
	Epoch   Epoch  `abi:"epoch"`
	Uptime  Uptime `abi:"uptime"`
}

// EncodeAbi encodes the commit epoch object to be placed in a transaction
func (c *CommitEpoch) EncodeAbi() ([]byte, error) {
	return commitEpochMethod.Encode(c)
}

// DecodeAbi decodes the commit epoch object from the given transaction
func (c *CommitEpoch) DecodeAbi(txData []byte) error {
	return decodeStruct(commitEpochMethod.Inputs, txData, &c)
}

// Type returns the state transaction type for given data
func (c *CommitEpoch) Type() StateTransactionType {
	return stTypeEndEpoch
}

func decodeStruct(t *abi.Type, input []byte, out interface{}) error {
	if len(input) < abiMethodIDLength {
		return fmt.Errorf("invalid commitment data, len = %d", len(input))
	}

	input = input[abiMethodIDLength:]

	val, err := abi.Decode(t, input)
	if err != nil {
		return err
	}

	metadata := &mapstructure.Metadata{}
	dc := &mapstructure.DecoderConfig{
		Result:     out,
		DecodeHook: customHook,
		TagName:    "abi",
		Metadata:   metadata,
	}

	ms, err := mapstructure.NewDecoder(dc)
	if err != nil {
		return err
	}

	if err = ms.Decode(val); err != nil {
		return err
	}

	if len(metadata.Unused) != 0 {
		return fmt.Errorf("some keys not used: %v", metadata.Unused)
	}

	return nil
}

var bigTyp = reflect.TypeOf(new(big.Int))

func customHook(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	if f == bigTyp && t.Kind() == reflect.Uint64 {
		// convert big.Int to uint64 (if possible)
		b, ok := data.(*big.Int)
		if !ok {
			return nil, fmt.Errorf("data not a big.Int")
		}

		if !b.IsUint64() {
			return nil, fmt.Errorf("cannot format big.Int to uint64")
		}

		return b.Uint64(), nil
	}

	return data, nil
}
