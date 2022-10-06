package polybft

import (
	"encoding/hex"
	"encoding/json"
	"math/big"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/contract"
)

var stateFunctions, _ = abi.NewABIFromList([]string{
	"function getValidators() returns (tuple(address,bytes)[])",
	"function getEpoch() returns (uint64)",
	"function init(tuple(address, uint256[4])[], uint64)",
})

var sidechainBridgeFunctions, _ = abi.NewABIFromList([]string{
	"function getNextExecutionIndex() returns (uint64)",
	"function getNextCommittedIndex() returns (uint64)",
})

// SystemState is an interface to interact with the consensus system contracts in the chain
type SystemState interface {
	// GetValidatorSet retrieves current validator set from the smart contract
	GetValidatorSet() (AccountSet, error)
	// GetEpoch retrieves current epoch number from the smart contract
	GetEpoch() (uint64, error)
	// GetNextExecutionIndex retrieves next bridge state sync index
	GetNextExecutionIndex() (uint64, error)
	// GetNextCommittedIndex retrieves next committed bridge state sync index
	GetNextCommittedIndex() (uint64, error)
}

var _ SystemState = &SystemStateImpl{}

// SystemStateImpl is implementation of SystemState interface
type SystemStateImpl struct {
	validatorContract       *contract.Contract
	sidechainBridgeContract *contract.Contract
}

// NewSystemState initializes new instance of systemState which abstracts smart contracts functions
func NewSystemState(config *PolyBFTConfig, provider contract.Provider) *SystemStateImpl {
	s := &SystemStateImpl{}
	s.validatorContract = contract.NewContract(
		ethgo.Address(config.ValidatorSetAddr),
		stateFunctions, contract.WithProvider(provider),
	)
	s.sidechainBridgeContract = contract.NewContract(
		ethgo.Address(config.SidechainBridgeAddr),
		sidechainBridgeFunctions,
		contract.WithProvider(provider),
	)
	return s
}

func (s *SystemStateImpl) InitValidatorSet(validators []*Validator, validatorSetSize int) error {
	validatorCons := make([][]byte, 0, len(validators))

	validatorType := abi.MustNewType("tuple(address ecdsa, uint256[4] bls)")
	for _, validator := range validators {
		blsKey, err := hex.DecodeString(validator.BlsKey)
		if err != nil {
			return err
		}

		pubKey, err := bls.UnmarshalPublicKey(blsKey)
		if err != nil {
			return err
		}

		blsBigInts, err := pubKey.ToBigInt()
		if err != nil {
			return err
		}

		// blsKey, err = abi.Encode(blsBigInts, abi.MustNewType("uint256[4]"))
		// if err != nil {
		// 	return err
		// }

		encoded, err := validatorType.Encode(map[string]interface{}{
			"ecdsa": validator.Address,
			"bls":   blsBigInts,
		})
		if err != nil {
			return err
		}

		validatorCons = append(validatorCons, encoded)
	}

	_, err := s.validatorContract.Call("init", ethgo.Latest, [2]interface{}{validatorCons, uint64(validatorSetSize)})

	return err
}

// GetValidatorSet retrieves current validator set from the smart contract
func (s *SystemStateImpl) GetValidatorSet() (AccountSet, error) {
	ret, err := s.validatorContract.Call("getValidators", ethgo.Latest)
	if err != nil {
		return nil, err
	}

	res := []*ValidatorAccount{}
	for _, i := range ret["0"].([]map[string]interface{}) {
		tmp := i["1"].([]byte)
		keyParts, err := abi.Decode(abi.MustNewType("uint256[]"), tmp)
		if err != nil {
			return nil, err
		}
		bigKey := keyParts.([4]*big.Int)
		blsKey, err := bls.UnmarshalPublicKeyFromBigInt(bigKey)
		if err != nil {
			return nil, err
		}

		res = append(res, &ValidatorAccount{
			Address: types.Address(i["0"].(ethgo.Address)),
			BlsKey:  blsKey,
		})
	}
	return AccountSet(res), nil
}

// GetEpoch retrieves current epoch number from the smart contract
func (s *SystemStateImpl) GetEpoch() (uint64, error) {
	rawResult, err := s.validatorContract.Call("getEpoch", ethgo.Latest)
	if err != nil {
		return 0, err
	}

	return rawResult["0"].(uint64), nil
}

// GetNextExecutionIndex retrieves next bridge state sync index
func (s *SystemStateImpl) GetNextExecutionIndex() (uint64, error) {
	rawResult, err := s.sidechainBridgeContract.Call("getNextExecutionIndex", ethgo.Latest)
	if err != nil {
		return 0, err
	}
	return rawResult["0"].(uint64), nil
}

// GetNextCommittedIndex retrieves next committed bridge state sync index
func (s *SystemStateImpl) GetNextCommittedIndex() (uint64, error) {
	rawResult, err := s.sidechainBridgeContract.Call("getNextCommittedIndex", ethgo.Latest)
	if err != nil {
		return 0, err
	}
	return rawResult["0"].(uint64), nil
}

func buildLogsFromReceipts(entry []*types.Receipt, header *types.Header) []*types.Log {
	var logs []*types.Log
	for _, taskReceipt := range entry {
		for _, taskLog := range taskReceipt.Logs {
			log := new(types.Log)
			*log = *taskLog

			data := map[string]interface{}{
				"Hash":   header.Hash,
				"Number": header.Number,
			}
			log.Data, _ = json.Marshal(&data)
			logs = append(logs, log)
		}
	}
	return logs
}
