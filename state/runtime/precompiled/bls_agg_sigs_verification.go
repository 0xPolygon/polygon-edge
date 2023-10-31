package precompiled

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

var (
	errBLSVerifyAggSignsInputs    = errors.New("invalid input")
	errBLSVerifyAggSignsInputsLen = errors.New("invalid input length")
	errBLSVInvalidSign            = errors.New("invalid signature")
	errBLSVInvalidPubKeys         = errors.New("invalid public keys")
	errBLSVInvalidMsg             = errors.New("invalid message")
	errBLSVInvalidBlsPart         = errors.New("bls verification part not in correct format")
	errBLSVInvalidPubKeysPart     = errors.New("could not find public keys part")
	// BlsVerificationABIType is ABI type used for BLS signatures verification.
	// It includes BLS public keys and bitmap representing signer validator accounts.
	BlsVerificationABIType = abi.MustNewType("tuple(bytes[], bytes)")
	// inputDataABIType is the ABI signature of the precompiled contract input data
	inputDataABIType = abi.MustNewType("tuple(bytes32, bytes, bytes)")
)

// blsAggSignsVerification verifies the given aggregated signatures using the default BLS utils functions.
// blsAggSignsVerification returns ABI encoded boolean value depends on validness of the given signatures.
type blsAggSignsVerification struct {
}

// gas returns the gas required to execute the pre-compiled contract
func (c *blsAggSignsVerification) gas(input []byte, _ *chain.ForksInTime) uint64 {
	return 150000
}

// Run runs the precompiled contract with the given input.
// Input must be ABI encoded: tuple(bytes, bytes[], bytes)
// Output could be an error or ABI encoded "bool" value
func (c *blsAggSignsVerification) run(input []byte, caller types.Address, host runtime.Host) ([]byte, error) {
	rawData, err := abi.Decode(inputDataABIType, input)
	if err != nil {
		return nil, err
	}

	data, ok := rawData.(map[string]interface{})
	if !ok {
		return nil, errBLSVerifyAggSignsInputs
	}

	if len(data) != 3 {
		return nil, errBLSVerifyAggSignsInputsLen
	}

	msg, ok := data["0"].([types.HashLength]byte)
	if !ok {
		return nil, errBLSVInvalidMsg
	}

	aggSig, ok := data["1"].([]byte)
	if !ok {
		return nil, errBLSVInvalidSign
	}

	sig, err := bls.UnmarshalSignature(aggSig)
	if err != nil {
		return nil, errBLSVInvalidSign
	}

	blsVerification, ok := data["2"].([]byte)
	if !ok {
		return nil, errBLSVInvalidPubKeys
	}

	decoded, err := BlsVerificationABIType.Decode(blsVerification)
	if err != nil {
		return nil, err
	}

	blsMap, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, errBLSVInvalidBlsPart
	}

	publicKeys, isOk := blsMap["0"].([][]byte)
	if !isOk {
		return nil, errBLSVInvalidPubKeysPart
	}

	blsPubKeys := make([]*bls.PublicKey, len(publicKeys))

	for i, pk := range publicKeys {
		blsPubKey, err := bls.UnmarshalPublicKey(pk)
		if err != nil {
			return nil, errBLSVInvalidPubKeys
		}

		blsPubKeys[i] = blsPubKey
	}

	if sig.VerifyAggregated(blsPubKeys, types.Hash(msg).Bytes(), signer.DomainStateReceiver) {
		return abiBoolTrue, nil
	}

	return abiBoolFalse, nil
}
