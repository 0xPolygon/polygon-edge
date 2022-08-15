package contract

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/valset"
	"github.com/hashicorp/go-hclog"
)

var (
	ErrSignerNotFound = errors.New("signer not found")
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
) valset.ValidatorSet {
	return &ContractValidatorSet{
		logger:     logger,
		blockchain: blockchain,
		executor:   executor,
		getSigner:  getSigner,
		epochSize:  epochSize,
	}
}

func (s *ContractValidatorSet) SourceType() valset.SourceType {
	return valset.Contract
}

func (s *ContractValidatorSet) Initialize() error {
	return nil
}

func (s *ContractValidatorSet) GetValidators(height uint64) (validators.Validators, error) {
	fetchingHeight := calculateFetchingHeight(height, s.epochSize)

	fetchedHeader, ok := s.blockchain.GetHeaderByNumber(fetchingHeight)
	if !ok {
		return nil, fmt.Errorf("header not found at %d", fetchingHeight)
	}

	signer, err := s.getSigner(height)
	if err != nil {
		return nil, err
	}

	if signer == nil {
		return nil, ErrSignerNotFound
	}

	transition, err := s.executor.BeginTxn(fetchedHeader.StateRoot, fetchedHeader, types.ZeroAddress)
	if err != nil {
		return nil, err
	}

	return FetchValidators(signer.Type(), transition)
}

func calculateFetchingHeight(usingHeight, epochSize uint64) uint64 {
	beginningEpoch := (usingHeight / epochSize) * epochSize

	// Determine the height of the end of the last epoch
	if beginningEpoch == 0 {
		return 0
	}

	// the end of the last epoch
	return beginningEpoch - 1
}
