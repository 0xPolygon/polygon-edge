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
		epochSize:  epochSize,
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

	// Determine the height of the end of the last epoch
	fetchedHeight := uint64(0)
	if beginningEpoch > 0 {
		fetchedHeight = beginningEpoch - 1
	}

	fetchedHeader, ok := s.blockchain.GetHeaderByNumber(fetchedHeight)
	if !ok {
		return nil, fmt.Errorf("header not found at %d", fetchedHeight)
	}

	signer, err := s.getSigner(fetchedHeight)
	if err != nil {
		return nil, err
	}

	if signer == nil {
		return nil, fmt.Errorf("signer not found")
	}

	transition, err := s.executor.BeginTxn(fetchedHeader.StateRoot, fetchedHeader, types.ZeroAddress)
	if err != nil {
		return nil, err
	}

	return FetchValidators(signer.Type(), transition, signer.Address())
}
