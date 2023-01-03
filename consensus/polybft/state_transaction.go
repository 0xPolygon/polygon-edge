package polybft

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state/runtime/precompiled"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/mitchellh/mapstructure"
	"github.com/umbracle/ethgo/abi"
)

var (
	commitmentABIType = abi.MustNewType("tuple(uint256 startId, uint256 endId, bytes32 root)")

	commitABIMethod, _ = abi.NewMethod("function commit(" +
		"tuple(uint256 startId, uint256 endId, bytes32 root) commitment," +
		"bytes signature," +
		"bytes bitmap)")
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

// Commitment holds merkle trie of bridge transactions accompanied by epoch number
type Commitment struct {
	MerkleTree *MerkleTree
	Epoch      uint64
	FromIndex  uint64
	ToIndex    uint64
}

// NewCommitment creates a new commitment object
func NewCommitment(epoch uint64, stateSyncEvents []*types.StateSyncEvent) (*Commitment, error) {
	tree, err := createMerkleTree(stateSyncEvents)
	if err != nil {
		return nil, err
	}

	return &Commitment{
		MerkleTree: tree,
		Epoch:      epoch,
		FromIndex:  stateSyncEvents[0].ID,
		ToIndex:    stateSyncEvents[len(stateSyncEvents)-1].ID,
	}, nil
}

// Hash calculates hash value for commitment object.
func (cm *Commitment) Hash() (types.Hash, error) {
	commitment := map[string]interface{}{
		"startId": cm.FromIndex,
		"endId":   cm.ToIndex,
		"root":    cm.MerkleTree.Hash(),
	}

	data, err := commitmentABIType.Encode(commitment)
	if err != nil {
		return types.Hash{}, err
	}

	return crypto.Keccak256Hash(data), nil
}

// CommitmentMessage holds metadata for bridge transactions
type CommitmentMessage struct {
	MerkleRootHash types.Hash
	FromIndex      uint64
	ToIndex        uint64
	Epoch          uint64
}

// NewCommitmentMessage creates a new commitment message based on provided merkle root hash
// where fromIndex represents an id of the first state event index in commitment
// where toIndex represents an id of the last state event index in commitment
func NewCommitmentMessage(merkleRootHash types.Hash, fromIndex, toIndex uint64) *CommitmentMessage {
	return &CommitmentMessage{
		MerkleRootHash: merkleRootHash,
		FromIndex:      fromIndex,
		ToIndex:        toIndex,
	}
}

// Hash calculates hash value for commitment object.
func (cm *CommitmentMessage) Hash() (types.Hash, error) {
	commitment := map[string]interface{}{
		"startId": cm.FromIndex,
		"endId":   cm.ToIndex,
		"root":    cm.MerkleRootHash,
	}

	data, err := commitmentABIType.Encode(commitment)
	if err != nil {
		return types.Hash{}, err
	}

	return crypto.Keccak256Hash(data), nil
}

// VerifyStateSyncProof validates given state sync proof
// against merkle trie root hash contained in the CommitmentMessage
func (cm CommitmentMessage) VerifyStateSyncProof(stateSyncProof *types.StateSyncProof) error {
	if stateSyncProof.StateSync == nil {
		return errors.New("no state sync event")
	}

	hash, err := stateSyncProof.StateSync.EncodeAbi()
	if err != nil {
		return err
	}

	return VerifyProof(stateSyncProof.StateSync.ID-cm.FromIndex, hash, stateSyncProof.Proof, cm.MerkleRootHash)
}

var _ StateTransactionInput = &CommitmentMessageSigned{}

// CommitmentMessageSigned encapsulates commitment message with aggregated signatures
type CommitmentMessageSigned struct {
	Message      *CommitmentMessage
	AggSignature Signature
	PublicKeys   [][]byte
}

// EncodeAbi contains logic for encoding arbitrary data into ABI format
func (cm *CommitmentMessageSigned) EncodeAbi() ([]byte, error) {
	commitment := map[string]interface{}{
		"startId": cm.Message.FromIndex,
		"endId":   cm.Message.ToIndex,
		"root":    cm.Message.MerkleRootHash,
	}

	blsVerificationPart, err := precompiled.BlsVerificationABIType.Encode(
		[2]interface{}{cm.PublicKeys, cm.AggSignature.Bitmap})
	if err != nil {
		return nil, err
	}

	data := map[string]interface{}{
		"commitment": commitment,
		"signature":  cm.AggSignature.AggregatedSignature,
		"bitmap":     blsVerificationPart,
	}

	return commitABIMethod.Encode(data)
}

// DecodeAbi contains logic for decoding given ABI data
func (cm *CommitmentMessageSigned) DecodeAbi(txData []byte) error {
	if len(txData) < abiMethodIDLength {
		return fmt.Errorf("invalid commitment data, len = %d", len(txData))
	}

	rawResult, err := commitABIMethod.Inputs.Decode(txData[abiMethodIDLength:])
	if err != nil {
		return err
	}

	result, isOk := rawResult.(map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not convert decoded data to map")
	}

	commitmentPart, isOk := result["commitment"].(map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find commitment part")
	}

	aggregatedSignature, isOk := result["signature"].([]byte)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find signature part")
	}

	blsVerificationPart, isOk := result["bitmap"].([]byte)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find bls verification part")
	}

	decoded, err := precompiled.BlsVerificationABIType.Decode(blsVerificationPart)
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

	merkleRoot, isOk := commitmentPart["root"].([types.HashLength]byte)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find merkle root hash")
	}

	fromIndexPart, isOk := commitmentPart["startId"].(*big.Int)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find startId")
	}

	fromIndex := fromIndexPart.Uint64()

	toIndexPart, isOk := commitmentPart["endId"].(*big.Int)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find endId")
	}

	toIndex := toIndexPart.Uint64()

	*cm = CommitmentMessageSigned{
		Message: NewCommitmentMessage(merkleRoot, fromIndex, toIndex),
		AggSignature: Signature{
			AggregatedSignature: aggregatedSignature,
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

	if bytes.Equal(sig, commitABIMethod.ID()) {
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
			!bytes.Equal(tx.Input[:abiMethodIDLength], commitABIMethod.ID()) {
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
