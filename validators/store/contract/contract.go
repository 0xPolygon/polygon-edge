package contract

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/hashicorp/go-hclog"
)

var (
	ErrSignerNotFound = errors.New("signer not found")
)

type ContractValidatorStore struct {
	logger     hclog.Logger
	blockchain store.HeaderGetter
	executor   Executor
	getSigner  store.SignerGetter
	epochSize  uint64
}

type Executor interface {
	BeginTxn(types.Hash, *types.Header, types.Address) (*state.Transition, error)
}

func NewContractValidatorStore(
	logger hclog.Logger,
	blockchain store.HeaderGetter,
	executor Executor,
	getSigner store.SignerGetter,
	epochSize uint64,
) store.ValidatorStore {
	return &ContractValidatorStore{
		logger:     logger,
		blockchain: blockchain,
		executor:   executor,
		getSigner:  getSigner,
		epochSize:  epochSize,
	}
}

func (s *ContractValidatorStore) SourceType() store.SourceType {
	return store.Contract
}

func (s *ContractValidatorStore) Initialize() error {
	return nil
}

func (s *ContractValidatorStore) GetValidators(height uint64) (validators.Validators, error) {
	signer, err := s.getSigner(height)
	if err != nil {
		return nil, err
	}

	if signer == nil {
		return nil, ErrSignerNotFound
	}

	transition, err := s.getTransitionForQuery(height)
	if err != nil {
		return nil, err
	}

	return FetchValidators(signer.Type(), transition, signer.Address())
}

func (s *ContractValidatorStore) getTransitionForQuery(height uint64) (*state.Transition, error) {
	header, ok := s.blockchain.GetHeaderByNumber(height)
	if !ok {
		return nil, fmt.Errorf("header not found at %d", height)
	}

	return s.executor.BeginTxn(header.StateRoot, header, types.ZeroAddress)
}
