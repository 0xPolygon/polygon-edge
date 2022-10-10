package polybft

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/contract"
)

var stateFunctions, _ = abi.NewABIFromList([]string{
	"function getValidators() returns (tuple(address,uint256[4])[])",
	"function getEpoch() returns (uint64)",
	"function init(tuple(address ecdsa, uint256[4] bls)[] _validators, uint64 _validatorSetSize)",
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
	validatorCons := make([]map[string]interface{}, len(validators))

	for i, validator := range validators {
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

		validatorCons[i] = map[string]interface{}{
			"ecdsa": validator.Address,
			"bls":   blsBigInts,
		}
	}

	_, err := s.validatorContract.Call("init", ethgo.Latest, validatorCons, validatorSetSize)

	return err
}

// GetValidatorSet retrieves current validator set from the smart contract
func (s *SystemStateImpl) GetValidatorSet() (AccountSet, error) {
	ret, err := s.validatorContract.Call("getValidators", ethgo.Latest)
	if err != nil {
		return nil, err
	}

	res := []*ValidatorAccount{}

	validatorsMap, isOk := ret["0"].([]map[string]interface{})
	if !isOk {
		return nil, fmt.Errorf("failed to decode validator set data")
	}

	for _, i := range validatorsMap {
		address, isOk := i["0"].(ethgo.Address)
		if !isOk {
			return nil, fmt.Errorf("failed to decode validator address")
		}

		bigKey, isOk := i["1"].([4]*big.Int)
		if !isOk {
			return nil, fmt.Errorf("failed to decode validator bls key")
		}

		blsKey, err := bls.UnmarshalPublicKeyFromBigInt(bigKey)

		if err != nil {
			return nil, err
		}

		res = append(res, &ValidatorAccount{
			Address: types.Address(address),
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

	epochNumber, isOk := rawResult["0"].(uint64)
	if !isOk {
		return 0, fmt.Errorf("failed to decode epoch")
	}

	return epochNumber, nil
}

// GetNextExecutionIndex retrieves next bridge state sync index
func (s *SystemStateImpl) GetNextExecutionIndex() (uint64, error) {
	rawResult, err := s.sidechainBridgeContract.Call("getNextExecutionIndex", ethgo.Latest)
	if err != nil {
		return 0, err
	}

	nextExecutionIndex, isOk := rawResult["0"].(uint64)
	if !isOk {
		return 0, fmt.Errorf("failed to decode next execution index")
	}

	return nextExecutionIndex, nil
}

// GetNextCommittedIndex retrieves next committed bridge state sync index
func (s *SystemStateImpl) GetNextCommittedIndex() (uint64, error) {
	rawResult, err := s.sidechainBridgeContract.Call("getNextCommittedIndex", ethgo.Latest)
	if err != nil {
		return 0, err
	}

	nextCommittedIndex, isOk := rawResult["0"].(uint64)
	if !isOk {
		return 0, fmt.Errorf("failed to decode next committed index")
	}

	return nextCommittedIndex, nil
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
