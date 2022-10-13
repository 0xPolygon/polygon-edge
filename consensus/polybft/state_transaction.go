package polybft

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var (
	stateSyncEventABIType = abi.MustNewType("tuple(uint64 id, address sender, address target, bytes data)[]")

	registerCommitmentABIMethod, _ = abi.NewMethod("function registerCommitment(" +
		"tuple(bytes32 merkleRoot, uint64 fromIndex, uint64 toIndex, uint64 bundleSize, uint64 epoch) commitment," +
		"tuple(bytes aggregatedSignature, bytes bitmap) signature)")

	executeBundleABIMethod, _ = abi.NewMethod("function executeBundle(" +
		"tuple(bytes32 hash, bytes value)[] proof, " +
		"tuple(uint64 id, address sender, address target, bytes data)[] stateSyncEvents)")

	validatorsUptimeMethod, _ = abi.NewMethod("function uptime(bytes data)")
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
	stTypeBridgeCommitment = "commitment"
	stTypeEndEpoch         = "end-epoch"
	stTypeBridgeBundle     = "bundle"
)

// Bundle is a type alias for slice of StateSyncEvents
type Bundle []*StateSyncEvent

var _ StateTransactionInput = &BundleProof{}

// BundleProof contains the proof of a bundle
type BundleProof struct {
	Proof      map[types.Hash][]byte
	StateSyncs Bundle
}

// ID returns identificator of bundle proof, which correspond to its first state sync id
func (bp *BundleProof) ID() uint64 { return bp.StateSyncs[0].ID }

// EncodeAbi contains logic for encoding arbitrary data into ABI format
func (bp *BundleProof) EncodeAbi() ([]byte, error) {
	// convert state sync events to map for abi encoding
	stateSyncEventsForEncoding := stateSyncEventsToAbiSlice(bp.StateSyncs)

	// convert proof to map for abi encoding
	proof := make([]map[string]interface{}, len(bp.Proof))
	i := 0

	for k, v := range bp.Proof {
		i++

		proof[i] = map[string]interface{}{
			"hash":  k,
			"value": v,
		}
	}

	data := map[string]interface{}{
		"stateSyncEvents": stateSyncEventsForEncoding,
		"proof":           proof,
	}

	return executeBundleABIMethod.Encode(data)
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

	stateSyncEventsEncoded, isOk := result["stateSyncEvents"].([]map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid bundle data")
	}

	proofEncoded, isOk := result["proof"].([]map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid bundle data")
	}

	stateSyncs := make([]*StateSyncEvent, len(stateSyncEventsEncoded))

	for i, sse := range stateSyncEventsEncoded {
		id, isOk := sse["id"].(uint64)
		if !isOk {
			return fmt.Errorf("type assertion failed on state sync (id field)")
		}

		sender, isOk := sse["sender"].(ethgo.Address)
		if !isOk {
			return fmt.Errorf("type assertion failed on state sync (sender field)")
		}

		target, isOk := sse["target"].(ethgo.Address)
		if !isOk {
			return fmt.Errorf("type assertion failed on state sync (target field)")
		}

		data, isOk := sse["data"].([]byte)
		if !isOk {
			return fmt.Errorf("type assertion failed on state sync (data field)")
		}

		stateSyncs[i] = &StateSyncEvent{
			ID:     id,
			Sender: sender,
			Target: target,
			Data:   data,
		}
	}

	proof := map[types.Hash][]byte{}

	for _, item := range proofEncoded {
		hash, isOk := item["hash"].([types.HashLength]byte)
		if !isOk {
			return fmt.Errorf("proof data type assertion failed on hash field")
		}

		value, isOk := item["value"].([]byte)
		if !isOk {
			return fmt.Errorf("proof data type assertion failed on value field")
		}

		proof[hash] = value
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

const (
	abiMethodIDLength = 4
)

// Commitment holds merkle trie of bridge transactions accompanied by epoch number
type Commitment struct {
	//MerkleTrie *merkletrie.MerkleTrie
	Epoch uint64
}

// NewCommitment initializes a new Commitment instance
func NewCommitment(epoch, bundleSize uint64, stateSyncEvents []*StateSyncEvent) (*Commitment, error) {
	// trie, err := createMerkleTrie(stateSyncEvents, bundleSize)
	// if err != nil {
	// 	return nil, err
	// }
	return &Commitment{
		//MerkleTrie: trie, TODO: Nemanja - find what to do here
		Epoch: epoch,
	}, nil
}

// Hash calculates hash value for commitment object.
// It is calculated as an aggregation of given merkle trie hash and epoch
func (cm *Commitment) Hash() types.Hash {
	// TODO: Nemanja - fix this
	//return commitmentHash(cm.MerkleTrie.Trie.Hash(), cm.Epoch)
	return commitmentHash(types.Hash{}, cm.Epoch)
}

// CommitmentMessage holds metadata for bridge transactions
type CommitmentMessage struct {
	MerkleRootHash types.Hash
	FromIndex      uint64
	ToIndex        uint64
	Epoch          uint64
	BundleSize     uint64
}

// NewCommitmentMessage initializes a CommitmentMessage instance
func NewCommitmentMessage(merkleRootHash types.Hash, fromIndex, toIndex, epoch, bundleSize uint64) *CommitmentMessage {
	return &CommitmentMessage{
		MerkleRootHash: merkleRootHash,
		FromIndex:      fromIndex,
		ToIndex:        toIndex,
		Epoch:          epoch,
		BundleSize:     bundleSize,
	}
}

// Hash calculates hash value for commitment object.
// It is calculated as an aggregation of given merkle trie root hash and epoch
func (cm *CommitmentMessage) Hash() types.Hash {
	return commitmentHash(cm.MerkleRootHash, cm.Epoch)
}

// GetBundleIdxFromStateSyncEventIdx resolves bundle index based on given state sync event index
func (cm *CommitmentMessage) GetBundleIdxFromStateSyncEventIdx(stateSyncEventIdx uint64) uint64 {
	return (stateSyncEventIdx - cm.FromIndex) / cm.BundleSize
}

// GetFirstStateSyncIndexFromBundleIndex returns first state sync index based on bundle size and given bundle index
// (offseted by FromIndex in CommitmentMessage)
func (cm *CommitmentMessage) GetFirstStateSyncIndexFromBundleIndex(bundleIndex uint64) uint64 {
	return (cm.BundleSize * bundleIndex) + cm.FromIndex
}

// ContainsStateSync checks whether CommitmentMessage contains state sync event identified by index,
// by comparing given state sync index with the bounds of CommitmentMessage
func (cm *CommitmentMessage) ContainsStateSync(stateSyncIndex uint64) bool {
	return stateSyncIndex >= cm.FromIndex && stateSyncIndex <= cm.ToIndex
}

// BundlesCount calculates bundles count contained in given CommitmentMessge
func (cm *CommitmentMessage) BundlesCount() uint64 {
	return (cm.ToIndex - cm.FromIndex + cm.BundleSize) / cm.BundleSize
}

// VerifyProof validates given bundle proof against merkle trie root hash contained in the CommitmentMessage
func (cm CommitmentMessage) VerifyProof(bundle *BundleProof) error {
	// TO DO Nemanja - always verify until fixed
	/*
		if len(bundle.StateSyncs) == 0 {
			return errors.New("no state sync events")
		}
		bundleIndx := cm.GetBundleIdxFromStateSyncEventIdx(bundle.StateSyncs[0].ID)
		key, err := rlp.EncodeToBytes(uint(bundleIndx))
		if err != nil {
			return err
		}

		pdb := rawdb.NewMemoryDatabase()
		for k, v := range bundle.Proof {
			err = pdb.Put(k.Bytes(), v)
			if err != nil {
				return err
			}
		}
		value, err := trie.VerifyProof(cm.MerkleRootHash, key, pdb)
		if err != nil {
			return err
		}

		hash, err := stateSyncEventsToHash(bundle.StateSyncs)
		if err != nil {
			return err
		}
		if !bytes.Equal(value, hash) {
			return fmt.Errorf("invalid proof for bundle %v, stateSyncHash: %v, proofValue: %v",
				bundleIndx, hex.EncodeToString(hash), hex.EncodeToString(value))
		}

	*/
	return nil
}

var _ StateTransactionInput = &CommitmentMessageSigned{}

// CommitmentMessageSigned encapsulates commitment message with aggregated signatures
type CommitmentMessageSigned struct {
	Message      *CommitmentMessage
	AggSignature Signature
}

// EncodeAbi contains logic for encoding arbitrary data into ABI format
func (cm *CommitmentMessageSigned) EncodeAbi() ([]byte, error) {
	commitment := map[string]interface{}{
		"merkleRoot": cm.Message.MerkleRootHash,
		"fromIndex":  cm.Message.FromIndex,
		"toIndex":    cm.Message.ToIndex,
		"bundleSize": cm.Message.BundleSize,
		"epoch":      cm.Message.Epoch,
	}
	signature := map[string]interface{}{
		"aggregatedSignature": cm.AggSignature.AggregatedSignature,
		"bitmap":              cm.AggSignature.Bitmap,
	}

	return registerCommitmentABIMethod.Encode([2]interface{}{commitment, signature})
}

// DecodeAbi contains logic for decoding given ABI data
func (cm *CommitmentMessageSigned) DecodeAbi(txData []byte) error {
	if len(txData) < abiMethodIDLength {
		return fmt.Errorf("invalid bundle data, len = %d", len(txData))
	}

	rawResult, err := registerCommitmentABIMethod.Inputs.Decode(txData[abiMethodIDLength:])
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

	signaturePart, isOk := result["signature"].(map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find signature part")
	}

	merkleRoot, isOk := commitmentPart["merkleRoot"].([types.HashLength]byte)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find merkle root hash")
	}

	fromIndex, isOk := commitmentPart["fromIndex"].(uint64)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find fromIndex")
	}

	toIndex, isOk := commitmentPart["toIndex"].(uint64)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find toIndex")
	}

	bundleSize, isOk := commitmentPart["bundleSize"].(uint64)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find bundle size")
	}

	epoch, isOk := commitmentPart["epoch"].(uint64)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not find epoch")
	}

	aggregatedSignature, isOk := signaturePart["aggregatedSignature"].([]byte)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could convert aggregated signature")
	}

	bitmap, isOk := signaturePart["bitmap"].([]byte)
	if !isOk {
		return fmt.Errorf("invalid commitment data. Could not convert bitmap")
	}

	*cm = CommitmentMessageSigned{
		Message: NewCommitmentMessage(merkleRoot, fromIndex, toIndex, epoch, bundleSize),
		AggSignature: Signature{
			AggregatedSignature: aggregatedSignature,
			Bitmap:              bitmap,
		},
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

	if bytes.Equal(sig, registerCommitmentABIMethod.ID()) {
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
			"id":     sse.ID,
			"sender": sse.Sender,
			"target": sse.Target,
			"data":   sse.Data,
		}
	}

	return result
}

// func stateSyncEventsToHash(stateSyncEvents []*StateSyncEvent) ([]byte, error) {
// 	stateSyncEventsForEncoding := stateSyncEventsToAbiSlice(stateSyncEvents)
// 	stateSyncEncoded, err := stateSyncEventABIType.Encode(stateSyncEventsForEncoding)
// 	if err != nil {
// 		return nil, err
// 	}

// 	//return crypto.Keccak256(stateSyncEncoded), nil

// 	return crypto.Keccak256Hash
// }

// func createMerkleTrie(stateSyncEvents []*StateSyncEvent, bundleSize uint64) (*merkletrie.MerkleTrie, error) {
// 	bundlesCount := (uint64(len(stateSyncEvents)) + bundleSize - 1) / bundleSize
// 	bundles := make([][]byte, bundlesCount)
// 	for i := uint64(0); i < bundlesCount; i++ {
// 		from, until := i*bundleSize, (i+1)*bundleSize
// 		if until > uint64(len(stateSyncEvents)) {
// 			until = uint64(len(stateSyncEvents))
// 		}
// 		hash, err := stateSyncEventsToHash(stateSyncEvents[from:until])
// 		if err != nil {
// 			return nil, err
// 		}
// 		bundles[i] = hash
// 	}

// 	return merkletrie.NewMerkleTrie(bundles)
// }

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
