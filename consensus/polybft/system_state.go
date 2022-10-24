package polybft

import (
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
	"function currentEpochId() returns (uint256)",
	"function getCurrentValidatorSet() returns (address[])",
	"function getValidator(address) returns (tuple(uint256[4],uint256,uint256,uint256))",
})

var sidechainBridgeFunctions, _ = abi.NewABIFromList([]string{
	"function counter() returns (uint256)",
	"function lastCommittedId() returns (uint256)",
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
		ethgo.Address(config.StateReceiverAddr),
		sidechainBridgeFunctions,
		contract.WithProvider(provider),
	)

	return s
}

// GetValidatorSet retrieves current validator set from the smart contract
func (s *SystemStateImpl) GetValidatorSet() (AccountSet, error) {
	output, err := s.validatorContract.Call("getCurrentValidatorSet", ethgo.Latest)
	if err != nil {
		return nil, err
	}

	res := AccountSet{}

	addresses, isOk := output["0"].([]ethgo.Address)
	if !isOk {
		return nil, fmt.Errorf("failed to decode addresses of the current validator set")
	}

	queryValidator := func(addr ethgo.Address) (*ValidatorAccount, error) {
		output, err := s.validatorContract.Call("getValidator", ethgo.Latest, addr)
		if err != nil {
			return nil, err
		}

		output, isOk = output["0"].(map[string]interface{})
		if !isOk {
			return nil, fmt.Errorf("failed to decode validator data")
		}

		pubKey, err := bls.UnmarshalPublicKeyFromBigInt(output["0"].([4]*big.Int))
		if err != nil {
			return nil, err
		}

		val := &ValidatorAccount{
			Address: types.Address(addr),
			BlsKey:  pubKey,
		}

		return val, nil
	}

	for _, index := range addresses {
		val, err := queryValidator(index)
		if err != nil {
			return nil, err
		}

		res = append(res, val)
	}

	return res, nil
}

// GetEpoch retrieves current epoch number from the smart contract
func (s *SystemStateImpl) GetEpoch() (uint64, error) {
	rawResult, err := s.validatorContract.Call("currentEpochId", ethgo.Latest)
	if err != nil {
		return 0, err
	}

	epochNumber, isOk := rawResult["0"].(*big.Int)
	if !isOk {
		return 0, fmt.Errorf("failed to decode epoch")
	}

	return epochNumber.Uint64(), nil
}

// GetNextExecutionIndex retrieves next bridge state sync index
func (s *SystemStateImpl) GetNextExecutionIndex() (uint64, error) {
	rawResult, err := s.sidechainBridgeContract.Call("counter", ethgo.Latest)
	if err != nil {
		return 0, err
	}

	nextExecutionIndex, isOk := rawResult["0"].(*big.Int)
	if !isOk {
		return 0, fmt.Errorf("failed to decode next execution index")
	}

	return nextExecutionIndex.Uint64() + 1, nil
}

// GetNextCommittedIndex retrieves next committed bridge state sync index
func (s *SystemStateImpl) GetNextCommittedIndex() (uint64, error) {
	rawResult, err := s.sidechainBridgeContract.Call("lastCommittedId", ethgo.Latest)
	if err != nil {
		return 0, err
	}

	nextCommittedIndex, isOk := rawResult["0"].(*big.Int)
	if !isOk {
		return 0, fmt.Errorf("failed to decode next committed index")
	}

	return nextCommittedIndex.Uint64() + 1, nil
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
