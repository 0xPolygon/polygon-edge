package contract

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
)

const (
	// How many validator sets are stored in the cache
	// Cache 3 validator sets for 3 epochs
	DefaultValidatorSetCacheSize = 3
)

var (
	ErrSignerNotFound                 = errors.New("signer not found")
	ErrInvalidValidatorsTypeAssertion = errors.New("invalid type assertion for Validators")
)

type ContractValidatorStore struct {
	logger     hclog.Logger
	blockchain store.HeaderGetter
	executor   Executor

	// LRU cache for the validators
	validatorSetCache *lru.Cache
}

type Executor interface {
	BeginTxn(types.Hash, *types.Header, types.Address) (*state.Transition, error)
}

func NewContractValidatorStore(
	logger hclog.Logger,
	blockchain store.HeaderGetter,
	executor Executor,
	validatorSetCacheSize int,
) (*ContractValidatorStore, error) {
	var (
		validatorsCache *lru.Cache
		err             error
	)

	if validatorSetCacheSize > 0 {
		if validatorsCache, err = lru.New(validatorSetCacheSize); err != nil {
			return nil, fmt.Errorf("unable to create validator set cache, %w", err)
		}
	}

	return &ContractValidatorStore{
		logger:            logger,
		blockchain:        blockchain,
		executor:          executor,
		validatorSetCache: validatorsCache,
	}, nil
}

func (s *ContractValidatorStore) SourceType() store.SourceType {
	return store.Contract
}

func (s *ContractValidatorStore) GetValidatorsByHeight(
	validatorType validators.ValidatorType,
	height uint64,
) (validators.Validators, error) {
	cachedValidators, err := s.loadCachedValidatorSet(height)
	if err != nil {
		return nil, err
	}

	if cachedValidators != nil {
		return cachedValidators, nil
	}

	transition, err := s.getTransitionForQuery(height)
	if err != nil {
		return nil, err
	}

	fetchedValidators, err := FetchValidators(validatorType, transition, types.ZeroAddress)
	if err != nil {
		return nil, err
	}

	s.saveToValidatorSetCache(height, fetchedValidators)

	return fetchedValidators, nil
}

func (s *ContractValidatorStore) getTransitionForQuery(height uint64) (*state.Transition, error) {
	header, ok := s.blockchain.GetHeaderByNumber(height)
	if !ok {
		return nil, fmt.Errorf("header not found at %d", height)
	}

	return s.executor.BeginTxn(header.StateRoot, header, types.ZeroAddress)
}

// loadCachedValidatorSet loads validators from validatorSetCache
func (s *ContractValidatorStore) loadCachedValidatorSet(height uint64) (validators.Validators, error) {
	cachedRawValidators, ok := s.validatorSetCache.Get(height)
	if !ok {
		return nil, nil
	}

	validators, ok := cachedRawValidators.(validators.Validators)
	if !ok {
		return nil, ErrInvalidValidatorsTypeAssertion
	}

	return validators, nil
}

// saveToValidatorSetCache saves validators to validatorSetCache
func (s *ContractValidatorStore) saveToValidatorSetCache(height uint64, validators validators.Validators) bool {
	if s.validatorSetCache == nil {
		return false
	}

	return s.validatorSetCache.Add(height, validators)
}
