package polybft

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var (
	stateSyncEventABIType = abi.MustNewType(
		"tuple(tuple(uint256 id, address sender, address receiver, bytes data, bool skip)[])")

	bundleABIType = abi.MustNewType("tuple(uint256 startId, uint256 endId, uint256 leaves, bytes32 root)")

	commitBundleABIMethod, _ = abi.NewMethod("function commit(" +
		"tuple(uint256 startId, uint256 endId, uint256 leaves, bytes32 root) bundle," +
		"bytes signature," +
		"bytes bitmap)")

	executeBundleABIMethod, _ = abi.NewMethod("function execute(" +
		"bytes32[] proof, " +
		"tuple(uint256 id, address sender, address receiver, bytes data, bool skip)[] objs)")

	validatorsUptimeMethod, _ = abi.NewMethod("function uptime(bytes data)")

	// BlsVerificationABIType is ABI type used for BLS signatures verification.
	// It includes BLS public keys and bitmap representing signer validator accounts.
	// TODO - move this to precompile, once we implement it on edge
	BlsVerificationABIType = abi.MustNewType("tuple(bytes[], bytes)")
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

// StateTransactionType is a type, which represents state transaction type
type StateTransactionType string

const (
	abiMethodIDLength      = 4
	stTypeBridgeCommitment = "commitment"
	stTypeEndEpoch         = "end-epoch"
	stTypeBridgeBundle     = "bundle"
)

// Bundle is a type alias for slice of StateSyncEvents
type Bundle []*StateSyncEvent

var _ StateTransactionInput = &BundleProof{}

// BundleProof contains the proof of a bundle
type BundleProof struct {
	Proof      []types.Hash
	StateSyncs Bundle
}

// ID returns identificator of bundle proof, which correspond to its first state sync id
func (bp *BundleProof) ID() uint64 { return bp.StateSyncs[0].ID }

// EncodeAbi contains logic for encoding arbitrary data into ABI format
func (bp *BundleProof) EncodeAbi() ([]byte, error) {
	return executeBundleABIMethod.Encode([2]interface{}{bp.Proof, stateSyncEventsToAbiSlice(bp.StateSyncs)})
}

// DecodeAbi contains logic for decoding given ABI data
func (bp *BundleProof) DecodeAbi(txData []byte) error {
	if len(txData) < abiMethodIDLength {
		return fmt.Errorf("invalid bundle data, len = %d", len(txData))
	}
	rawResult, err := executeBundleABIMethod.Inputs.Decode(txData[abiMethodIDLength:])
	if err != nil {
		return err
	}

	result, isOk := rawResult.(map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid bundle data")
	}
	stateSyncEventsEncoded, isOk := result["objs"].([]map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid state sync data")
	}
	proofEncoded, isOk := result["proof"].([][32]byte)
	if !isOk {
		return fmt.Errorf("invalid proof data")
	}

	stateSyncs := make([]*StateSyncEvent, len(stateSyncEventsEncoded))
	for i, sse := range stateSyncEventsEncoded {
		stateSyncs[i] = &StateSyncEvent{
			ID:       (sse["id"].(*big.Int)).Uint64(),
			Sender:   sse["sender"].(ethgo.Address),
			Receiver: sse["receiver"].(ethgo.Address),
			Data:     sse["data"].([]byte),
			Skip:     sse["skip"].(bool),
		}
	}

	proof := make([]types.Hash, len(proofEncoded))
	for i := 0; i < len(proofEncoded); i++ {
		proof[i] = types.Hash(proofEncoded[i])
	}

	*bp = BundleProof{
		Proof:      proof,
		StateSyncs: stateSyncs,
	}
	return nil
}

// Type returns type of state transaction input
func (bp *BundleProof) Type() StateTransactionType {
	return stTypeBridgeBundle
}

// Commitment holds merkle trie of bridge transactions accompanied by epoch number
type Commitment struct {
	MerkleTree *MerkleTree
	Epoch      uint64
	FromIndex  uint64
	ToIndex    uint64
	LeavesNum  uint64
}

// NewCommitment creates a new commitment object
func NewCommitment(epoch, fromIndex, toIndex, bundleSize uint64,
	stateSyncEvents []*StateSyncEvent) (*Commitment, error) {
	tree, err := createMerkleTree(stateSyncEvents, bundleSize)
	if err != nil {
		return nil, err
	}

	return &Commitment{
		MerkleTree: tree,
		Epoch:      epoch,
		FromIndex:  fromIndex,
		ToIndex:    toIndex,
		LeavesNum:  (toIndex - fromIndex + bundleSize) / bundleSize,
	}, nil
}

// Hash calculates hash value for commitment object.
func (cm *Commitment) Hash() (types.Hash, error) {
	commitment := map[string]interface{}{
		"startId": cm.FromIndex,
		"endId":   cm.ToIndex,
		"leaves":  cm.LeavesNum,
		"root":    cm.MerkleTree.Hash(),
	}

	data, err := bundleABIType.Encode(commitment)
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
	BundleSize     uint64
}

// NewCommitmentMessage creates a new commitment message based on provided merkle root hash
// where fromIndex represents an id of the first state event index in commitment
// where toIndex represents an id of the last state event index in commitment
// where bundleSize represents the number of bundles (leafs) in commitment
func NewCommitmentMessage(merkleRootHash types.Hash, fromIndex, toIndex, bundleSize uint64) *CommitmentMessage {
	return &CommitmentMessage{
		MerkleRootHash: merkleRootHash,
		FromIndex:      fromIndex,
		ToIndex:        toIndex,
		BundleSize:     bundleSize,
	}
}

// Hash calculates hash value for commitment object.
func (cm *CommitmentMessage) Hash() (types.Hash, error) {
	commitment := map[string]interface{}{
		"startId": cm.FromIndex,
		"endId":   cm.ToIndex,
		"leaves":  cm.BundlesCount(),
		"root":    cm.MerkleRootHash,
	}

	data, err := bundleABIType.Encode(commitment)
	if err != nil {
		return types.Hash{}, err
	}

	return crypto.Keccak256Hash(data), nil
}

// GetBundleIdxFromStateSyncEventIdx resolves bundle index based on given state sync event index
func (cm *CommitmentMessage) GetBundleIdxFromStateSyncEventIdx(stateSyncEventIdx uint64) uint64 {
	return (stateSyncEventIdx - cm.FromIndex) / cm.BundleSize
}

// GetFirstStateSyncIndexFromBundleIndex returns first state sync index based on bundle size and given bundle index
// (offseted by FromIndex in CommitmentMessage)
func (cm *CommitmentMessage) GetFirstStateSyncIndexFromBundleIndex(bundleIndex uint64) uint64 {
	if bundleIndex == 0 {
		return cm.FromIndex
	}
	return (cm.BundleSize * bundleIndex) + cm.FromIndex
}

// ContainsStateSync checks whether CommitmentMessage contains state sync event identified by index,
// by comparing given state sync index with the bounds of CommitmentMessage
func (cm *CommitmentMessage) ContainsStateSync(stateSyncIndex uint64) bool {
	return stateSyncIndex >= cm.FromIndex && stateSyncIndex <= cm.ToIndex
}

// BundlesCount calculates bundles count contained in given CommitmentMessge
func (cm *CommitmentMessage) BundlesCount() uint64 {
	return uint64(math.Ceil((float64(cm.ToIndex) - float64(cm.FromIndex) + 1) / float64(cm.BundleSize)))
}

// VerifyProof validates given bundle proof against merkle trie root hash contained in the CommitmentMessage
func (cm CommitmentMessage) VerifyProof(bundle *BundleProof) error {
	if len(bundle.StateSyncs) == 0 {
		return errors.New("no state sync events")
	}

	hash, err := stateSyncEventsToHash(bundle.StateSyncs)
	if err != nil {
		return err
	}

	bundleIndex := cm.GetBundleIdxFromStateSyncEventIdx(bundle.StateSyncs[0].ID)
	return VerifyProof(bundleIndex, hash, bundle.Proof, cm.MerkleRootHash)
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
		"leaves":  cm.Message.BundlesCount(),
		"root":    cm.Message.MerkleRootHash,
	}

	blsVerificationPart, err := BlsVerificationABIType.Encode([2]interface{}{cm.PublicKeys, cm.AggSignature.Bitmap})
	if err != nil {
		return nil, err
	}

	data := map[string]interface{}{
		"bundle":    commitment,
		"signature": cm.AggSignature.AggregatedSignature,
		"bitmap":    blsVerificationPart,
	}

	return commitBundleABIMethod.Encode(data)
}

// DecodeAbi contains logic for decoding given ABI data
func (cm *CommitmentMessageSigned) DecodeAbi(txData []byte) error {
	if len(txData) < abiMethodIDLength {
		return fmt.Errorf("invalid bundle data, len = %d", len(txData))
	}

	rawResult, err := commitBundleABIMethod.Inputs.Decode(txData[abiMethodIDLength:])
	if err != nil {
		return err
	}

	result, isOk := rawResult.(map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not convert decoded data to map")
	}

	commitmentPart := result["bundle"].(map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find commitment part")
	}

	aggregatedSignature, isOk := result["signature"].([]byte)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find signature part")
	}

	blsVerificationPart := result["bitmap"].([]byte)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find bls verification part")
	}

	decoded, err := BlsVerificationABIType.Decode(blsVerificationPart)
	if err != nil {
		return err
	}

	blsMap, ok := decoded.(map[string]interface{})
	if !ok {
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

	leavesPart, isOk := commitmentPart["leaves"].(*big.Int)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find endId")
	}

	leavesNum := leavesPart.Uint64()

	*cm = CommitmentMessageSigned{
		Message: NewCommitmentMessage(merkleRoot, fromIndex, toIndex,
			uint64(math.Ceil((float64(toIndex)-float64(fromIndex)+1)/float64(leavesNum)))),
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

	if bytes.Equal(sig, commitBundleABIMethod.ID()) {
		// bridge commitment
		obj = &CommitmentMessageSigned{}
	} else if bytes.Equal(sig, executeBundleABIMethod.ID()) {
		// bundle proof
		obj = &BundleProof{}
	} else {
		return nil, fmt.Errorf("unknown state transaction")
	}

	if err := obj.DecodeAbi(txData); err != nil {
		return nil, err
	}

	return obj, nil
}

func stateSyncEventsToAbiSlice(stateSyncEvents []*StateSyncEvent) []map[string]interface{} {
	result := make([]map[string]interface{}, len(stateSyncEvents))
	for i, sse := range stateSyncEvents {
		result[i] = map[string]interface{}{
			"id":       sse.ID,
			"sender":   sse.Sender,
			"receiver": sse.Receiver,
			"data":     sse.Data,
			"skip":     sse.Skip,
		}
	}
	return result
}

func stateSyncEventsToHash(stateSyncEvents []*StateSyncEvent) ([]byte, error) {
	stateSyncEventsForEncoding := stateSyncEventsToAbiSlice(stateSyncEvents)
	stateSyncEncoded, err := stateSyncEventABIType.Encode([]interface{}{stateSyncEventsForEncoding})
	if err != nil {
		return nil, err
	}

	return stateSyncEncoded, nil
}

func createMerkleTree(stateSyncEvents []*StateSyncEvent, bundleSize uint64) (*MerkleTree, error) {
	bundlesCount := (uint64(len(stateSyncEvents)) + bundleSize - 1) / bundleSize
	bundles := make([][]byte, bundlesCount)
	for i := uint64(0); i < bundlesCount; i++ {
		from, until := i*bundleSize, (i+1)*bundleSize
		if until > uint64(len(stateSyncEvents)) {
			until = uint64(len(stateSyncEvents))
		}
		hash, err := stateSyncEventsToHash(stateSyncEvents[from:until])
		if err != nil {
			return nil, err
		}
		bundles[i] = hash
	}

	return NewMerkleTree(bundles)
}

func commitmentHash(merkleRootHash types.Hash, epoch uint64) types.Hash {
	data := [types.HashLength + 8]byte{}
	copy(data[:], merkleRootHash.Bytes())
	binary.BigEndian.PutUint64(data[types.HashLength:], epoch)

	return types.BytesToHash(crypto.Keccak256(data[:]))
}

var _ StateTransactionInput = &UptimeCounter{}

// UptimeCounter contains information about how many blocks signed each validator during an epoch
type UptimeCounter struct {
	validatorIndices map[ethgo.Address]int
	validatorUptimes []*UptimeInfo
}

// AddUptime registers given validator, identified by address, to the uptime counter
func (u *UptimeCounter) AddUptime(address ethgo.Address) {
	if i, exists := u.validatorIndices[address]; exists {
		u.validatorUptimes[i].Count++
	} else {
		u.validatorUptimes = append(u.validatorUptimes, &UptimeInfo{Address: address, Count: 1})
		u.validatorIndices[address] = len(u.validatorUptimes) - 1
	}
}

// UptimeInfo contains validator address and count (denoting how many block given validator has signed)
type UptimeInfo struct {
	Address ethgo.Address
	Count   uint64
}

// EncodeAbi contains logic for encoding arbitrary data into ABI format
func (u *UptimeCounter) EncodeAbi() ([]byte, error) {
	uptime, err := json.Marshal(u.validatorUptimes)
	if err != nil {
		return nil, err
	}

	return validatorsUptimeMethod.Encode([1]interface{}{uptime})
}

// DecodeAbi contains logic for decoding given ABI data
func (u *UptimeCounter) DecodeAbi(txData []byte) error {
	if len(txData) < abiMethodIDLength {
		return fmt.Errorf("invalid bundle data, len = %d", len(txData))
	}

	raw, err := abi.Decode(validatorsUptimeMethod.Inputs, txData[abiMethodIDLength:])
	if err != nil {
		return err
	}

	resultMap, isOk := raw.(map[string]interface{})
	if !isOk {
		return fmt.Errorf("failed to decode uptime counter data")
	}

	bytes, isOk := resultMap["data"].([]byte)
	if !isOk {
		return fmt.Errorf("failed to decode uptime counter inner data")
	}

	var uptime []*UptimeInfo
	if err = json.Unmarshal(bytes, &uptime); err != nil {
		return err
	}

	u.validatorUptimes = uptime

	return nil
}

// Type returns type of state transaction input
func (u *UptimeCounter) Type() StateTransactionType {
	return stTypeEndEpoch
}
