package contract

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/valset"
	"github.com/hashicorp/go-hclog"
)

type ContractValidatorSet struct {
	logger     hclog.Logger
	blockchain valset.HeaderGetter
	executor   Executor
	getSigner  valset.SignerGetter
	epochSize  uint64
}

type Executor interface {
	BeginTxn(types.Hash, *types.Header, types.Address) (*state.Transition, error)
}

func NewContractValidatorSet(
	logger hclog.Logger,
	blockchain valset.HeaderGetter,
	executor Executor,
	getSigner valset.SignerGetter,
	epochSize uint64,
) (valset.ValidatorSet, error) {
	return &ContractValidatorSet{
		logger:     logger,
		blockchain: blockchain,
		executor:   executor,
		getSigner:  getSigner,
	}, nil
}

func (s *ContractValidatorSet) SourceType() valset.SourceType {
	return valset.Contract
}

func (s *ContractValidatorSet) Initialize() error {
	return nil
}

func (s *ContractValidatorSet) GetValidators(height uint64) (validators.Validators, error) {
	beginningEpoch := (height / s.epochSize) * s.epochSize
	beginningHeader, ok := s.blockchain.GetHeaderByNumber(beginningEpoch)

	if !ok {
		return nil, fmt.Errorf("header not found at %d", beginningEpoch)
	}

	signer, err := s.getSigner(beginningEpoch)
	if err != nil {
		return nil, err
	}

	if signer == nil {
		return nil, fmt.Errorf("signer not found")
	}

	validatorType := signer.Type()

	transition, err := s.executor.BeginTxn(beginningHeader.StateRoot, beginningHeader, types.ZeroAddress)
	if err != nil {
		return nil, err
	}

	return FetchValidators(validatorType, transition, signer.Address())
}
