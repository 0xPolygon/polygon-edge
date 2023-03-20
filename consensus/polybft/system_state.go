package polybft

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
)

// ValidatorInfo is data transfer object which holds validator information,
// provided by smart contract
type ValidatorInfo struct {
	Address             ethgo.Address
	Stake               *big.Int
	TotalStake          *big.Int
	Commission          *big.Int
	WithdrawableRewards *big.Int
	Active              bool
}

// SystemState is an interface to interact with the consensus system contracts in the chain
type SystemState interface {
	// GetValidatorSet retrieves current validator set from the smart contract
	GetValidatorSet() (AccountSet, error)
	// GetEpoch retrieves current epoch number from the smart contract
	GetEpoch() (uint64, error)
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
		contractsapi.ChildValidatorSet.Abi, contract.WithProvider(provider),
	)
	s.sidechainBridgeContract = contract.NewContract(
		ethgo.Address(config.StateReceiverAddr),
		contractsapi.StateReceiver.Abi,
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

	queryValidator := func(addr ethgo.Address) (*ValidatorMetadata, error) {
		output, err := s.validatorContract.Call("getValidator", ethgo.Latest, addr)
		if err != nil {
			return nil, fmt.Errorf("failed to call getValidator function: %w", err)
		}

		blsKey, ok := output["blsKey"].([4]*big.Int)
		if !ok {
			return nil, fmt.Errorf("failed to decode blskey")
		}

		pubKey, err := bls.UnmarshalPublicKeyFromBigInt(blsKey)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal BLS public key: %w", err)
		}

		totalStake, ok := output["totalStake"].(*big.Int)
		if !ok {
			return nil, fmt.Errorf("failed to decode total stake")
		}

		return &ValidatorMetadata{
			Address:     types.Address(addr),
			BlsKey:      pubKey,
			VotingPower: new(big.Int).Set(totalStake),
		}, nil
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
