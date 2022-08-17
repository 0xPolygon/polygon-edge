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

func (s *ContractValidatorSet) GetValidators(height, from uint64) (validators.Validators, error) {
	signer, err := s.getSigner(height)
	if err != nil {
		return nil, err
	}

	if signer == nil {
		return nil, ErrSignerNotFound
	}

	transition, err := s.getTransitionForQuery(height, from)
	if err != nil {
		return nil, err
	}

	return FetchValidators(signer.Type(), transition, signer.Address())
}

func (s *ContractValidatorSet) getTransitionForQuery(height uint64, from uint64) (*state.Transition, error) {
	fetchingHeight := calculateFetchingHeight(height, s.epochSize, from)

	header, ok := s.blockchain.GetHeaderByNumber(fetchingHeight)
	if !ok {
		return nil, fmt.Errorf("header not found at %d", fetchingHeight)
	}

	return s.executor.BeginTxn(header.StateRoot, header, types.ZeroAddress)
}

func calculateFetchingHeight(usingHeight, epochSize, from uint64) uint64 {
	beginningEpoch := (usingHeight / epochSize) * epochSize

	height := uint64(0)
	if beginningEpoch > 0 {
		// the end of the last epoch
		height = beginningEpoch - 1
	}

	if height <= from {
		if from == 0 {
			return from
		}

		return from - 1
	}

	return height
}
