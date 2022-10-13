package polybft

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/mitchellh/mapstructure"
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
		id, isOk := sse["id"].(*big.Int)
		if !isOk {
			return fmt.Errorf("invalid state sync event id")
		}

		sender, isOk := sse["sender"].(ethgo.Address)
		if !isOk {
			return fmt.Errorf("invalid state sync sender field")
		}

		receiver, isOk := sse["receiver"].(ethgo.Address)
		if !isOk {
			return fmt.Errorf("invalid state sync receiver field")
		}

		data, isOk := sse["data"].([]byte)
		if !isOk {
			return fmt.Errorf("invalid state sync data field")
		}

		skip, isOk := sse["skip"].(bool)
		if !isOk {
			return fmt.Errorf("invalid state sync skip field")
		}

		stateSyncs[i] = &StateSyncEvent{
			ID:       id.Uint64(),
			Sender:   sender,
			Receiver: receiver,
			Data:     data,
			Skip:     skip,
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

	commitmentPart, isOk := result["bundle"].(map[string]interface{})
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

	decoded, err := BlsVerificationABIType.Decode(blsVerificationPart)
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

var _ StateTransactionInput = &CommitEpoch{}

var (
	commitEpochMethod, _ = abi.NewMethod("function commitEpoch(" +
		// new epoch id
		"uint256 epochid," +
		// Epoch
		"tuple(uint256 startblock, uint256 endblock, bytes32 epochroot) epoch," +
		// Uptime
		"tuple(uint256 epochid,tuple(address validator,uint256 uptime)[] uptimedata,uint256 totaluptime) uptime)")
)

// Epoch holds the data about epoch execution (when it started and when it ended)
type Epoch struct {
	StartBlock uint64     `abi:"startblock"`
	EndBlock   uint64     `abi:"endblock"`
	EpochRoot  types.Hash `abi:"epochroot"`
}

// Uptime holds the data about number of times validators sealed blocks
// in a given epoch
type Uptime struct {
	EpochID     uint64            `abi:"epochid"`
	UptimeData  []ValidatorUptime `abi:"uptimedata"`
	TotalUptime uint64            `abi:"totaluptime"`
}

func (u *Uptime) addValidatorUptime(address types.Address, count uint64) {
	if u.UptimeData == nil {
		u.UptimeData = []ValidatorUptime{}
	}

	u.TotalUptime += count
	u.UptimeData = append(u.UptimeData, ValidatorUptime{
		Address: address,
		Count:   count,
	})
}

// ValidatorUptime contains data about how many blocks a given validator has sealed
// in a single period (epoch)
type ValidatorUptime struct {
	Address types.Address `abi:"validator"`
	Count   uint64        `abi:"uptime"`
}

// CommitEpoch contains data that is sent to ChildValidatorSet contract
// to distribute rewards on the end of an epoch
type CommitEpoch struct {
	EpochID uint64 `abi:"epochid"`
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
		return fmt.Errorf("invalid bundle data, len = %d", len(input))
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
