package fork

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/hook"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/0xPolygon/polygon-edge/validators/store/contract"
	"github.com/hashicorp/go-hclog"
)

const (
	loggerName                = "fork_manager"
	snapshotMetadataFilename  = "metadata"
	snapshotSnapshotsFilename = "snapshots"
)

var (
	ErrForkNotFound           = errors.New("fork not found")
	ErrSignerNotFound         = errors.New("signer not found")
	ErrValidatorStoreNotFound = errors.New("validator set not found")
	ErrKeyManagerNotFound     = errors.New("key manager not found")
)

// ValidatorStore is an interface that ForkManager calls for Validator Store
type ValidatorStore interface {
	store.ValidatorStore
	// Close defines termination process
	Close() error
	// GetValidators is a method to return validators at the given height
	GetValidators(height, epochSize, forkFrom uint64) (validators.Validators, error)
}

// HookRegister is an interface that ForkManager calls for hook registrations
type HooksRegister interface {
	// RegisterHooks register hooks for the given block height
	RegisterHooks(hooks *hook.Hooks, height uint64)
}

// HooksInterface is an interface of hooks to be called by IBFT
// This interface is referred from fork and ibft package
type HooksInterface interface {
	ShouldWriteTransactions(uint64) bool
	ModifyHeader(*types.Header, types.Address) error
	VerifyHeader(*types.Header) error
	VerifyBlock(*types.Block) error
	ProcessHeader(*types.Header) error
	PreCommitState(*types.Header, *state.Transition) error
	PostInsertBlock(*types.Block) error
}

// ForkManager is the module that has Fork configuration and multiple version of submodules
// and returns the proper submodule at specified height
type ForkManager struct {
	logger         hclog.Logger
	blockchain     store.HeaderGetter
	executor       contract.Executor
	secretsManager secrets.SecretsManager

	// configuration
	forks     IBFTForks
	filePath  string
	epochSize uint64

	// submodule lookup
	keyManagers     map[validators.ValidatorType]signer.KeyManager
	validatorStores map[store.SourceType]ValidatorStore
	hooksRegisters  map[IBFTType]HooksRegister
}

// NewForkManager is a constructor of ForkManager
func NewForkManager(
	logger hclog.Logger,
	blockchain store.HeaderGetter,
	executor contract.Executor,
	secretManager secrets.SecretsManager,
	filePath string,
	epochSize uint64,
	ibftConfig map[string]interface{},
) (*ForkManager, error) {
	forks, err := GetIBFTForks(ibftConfig)
	if err != nil {
		return nil, err
	}

	fm := &ForkManager{
		logger:          logger.Named(loggerName),
		blockchain:      blockchain,
		executor:        executor,
		secretsManager:  secretManager,
		filePath:        filePath,
		epochSize:       epochSize,
		forks:           forks,
		keyManagers:     make(map[validators.ValidatorType]signer.KeyManager),
		validatorStores: make(map[store.SourceType]ValidatorStore),
		hooksRegisters:  make(map[IBFTType]HooksRegister),
	}

	// Need initialization of signers in the constructor
	// because hash calculation is called from blockchain initialization
	if err := fm.initializeKeyManagers(); err != nil {
		return nil, err
	}

	return fm, nil
}

// Initialize initializes ForkManager on initialization phase
func (m *ForkManager) Initialize() error {
	if err := m.initializeValidatorStores(); err != nil {
		return err
	}

	m.initializeHooksRegisters()

	return nil
}

// Close calls termination process of submodules
func (m *ForkManager) Close() error {
	for _, store := range m.validatorStores {
		if err := store.Close(); err != nil {
			return err
		}
	}

	return nil
}

// GetSigner returns a proper signer at specified height
func (m *ForkManager) GetSigner(height uint64) (signer.Signer, error) {
	keyManager, err := m.getKeyManager(height)
	if err != nil {
		return nil, err
	}

	var parentKeyManager signer.KeyManager

	if height > 1 {
		if parentKeyManager, err = m.getKeyManager(height - 1); err != nil {
			return nil, err
		}
	}

	return signer.NewSigner(
		keyManager,
		parentKeyManager,
	), nil
}

// GetValidatorStore returns a proper validator set at specified height
func (m *ForkManager) GetValidatorStore(height uint64) (ValidatorStore, error) {
	fork := m.forks.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	set := m.getValidatorStoreByIBFTFork(fork)
	if set == nil {
		return nil, ErrValidatorStoreNotFound
	}

	return set, nil
}

// GetValidators returns validators at specified height
func (m *ForkManager) GetValidators(height uint64) (validators.Validators, error) {
	fork := m.forks.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	set := m.getValidatorStoreByIBFTFork(fork)
	if set == nil {
		return nil, ErrValidatorStoreNotFound
	}

	return set.GetValidators(
		height,
		m.epochSize,
		fork.From.Value,
	)
}

// GetHooks returns a hooks at specified height
func (m *ForkManager) GetHooks(height uint64) HooksInterface {
	hooks := &hook.Hooks{}

	for _, r := range m.hooksRegisters {
		r.RegisterHooks(hooks, height)
	}

	return hooks
}

func (m *ForkManager) getValidatorStoreByIBFTFork(fork *IBFTFork) ValidatorStore {
	set, ok := m.validatorStores[ibftTypesToSourceType[fork.Type]]
	if !ok {
		return nil
	}

	return set
}

func (m *ForkManager) getKeyManager(height uint64) (signer.KeyManager, error) {
	fork := m.forks.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	keyManager, ok := m.keyManagers[fork.ValidatorType]
	if !ok {
		return nil, ErrKeyManagerNotFound
	}

	return keyManager, nil
}

// initializeKeyManagers initialize all key managers based on Fork configuration
func (m *ForkManager) initializeKeyManagers() error {
	for _, fork := range m.forks {
		if err := m.initializeKeyManager(fork.ValidatorType); err != nil {
			return err
		}
	}

	return nil
}

// initializeKeyManager initializes the sp
func (m *ForkManager) initializeKeyManager(valType validators.ValidatorType) error {
	if _, ok := m.keyManagers[valType]; ok {
		return nil
	}

	keyManager, err := signer.NewKeyManagerFromType(m.secretsManager, valType)
	if err != nil {
		return err
	}

	m.keyManagers[valType] = keyManager

	return nil
}

// initializeValidatorStores initializes all validator sets based on Fork configuration
func (m *ForkManager) initializeValidatorStores() error {
	for _, fork := range m.forks {
		sourceType := ibftTypesToSourceType[fork.Type]
		if err := m.initializeValidatorStore(sourceType); err != nil {
			return err
		}
	}

	return nil
}

// initializeValidatorStore initializes the specified validator set
func (m *ForkManager) initializeValidatorStore(setType store.SourceType) error {
	if _, ok := m.validatorStores[setType]; ok {
		return nil
	}

	var (
		valStore ValidatorStore
		err      error
	)

	switch setType {
	case store.Snapshot:
		valStore, err = NewSnapshotValidatorStoreWrapper(
			m.logger,
			m.blockchain,
			m.GetSigner,
			m.filePath,
			m.epochSize,
		)
	case store.Contract:
		valStore, err = NewContractValidatorStoreWrapper(
			m.logger,
			m.blockchain,
			m.executor,
			m.GetSigner,
		)
	}

	if err != nil {
		return err
	}

	m.validatorStores[setType] = valStore

	return nil
}

// initializeHooksRegisters initialize all HookRegisters to be used
func (m *ForkManager) initializeHooksRegisters() {
	for _, fork := range m.forks {
		m.initializeHooksRegister(fork.Type)
	}
}

// initializeHooksRegister initialize HookRegister by IBFTType
func (m *ForkManager) initializeHooksRegister(ibftType IBFTType) {
	if _, ok := m.hooksRegisters[ibftType]; ok {
		return
	}

	switch ibftType {
	case PoA:
		m.hooksRegisters[PoA] = NewPoAHookRegisterer(
			m.getValidatorStoreByIBFTFork,
			m.forks,
		)
	case PoS:
		m.hooksRegisters[PoS] = NewPoSHookRegister(
			m.forks,
			m.epochSize,
		)
	}
}
