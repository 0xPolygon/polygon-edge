package fork

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/hook"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/secrets"
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
)

// ValidatorStore is an interface that HookManager calls for Validator Store
type ValidatorStore interface {
	store.ValidatorStore
	Closer
	ValidatorsGetter
}

// ForkManager is the module that has Fork configuration and multiple version of submodules
// and returns the proper submodule at specified height
type ForkManager struct {
	logger         hclog.Logger
	blockchain     store.HeaderGetter
	executor       contract.Executor
	secretsManager secrets.SecretsManager

	// configuration
	forks     []IBFTFork
	filePath  string
	epochSize uint64

	signers map[validators.ValidatorType]signer.Signer

	// validator sets
	validatorSets map[store.SourceType]ValidatorStore
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
		logger:         logger.Named(loggerName),
		blockchain:     blockchain,
		executor:       executor,
		secretsManager: secretManager,
		filePath:       filePath,
		epochSize:      epochSize,
		forks:          forks,
		signers:        make(map[validators.ValidatorType]signer.Signer),
		validatorSets:  make(map[store.SourceType]ValidatorStore),
	}

	// Need initialization of signers in the constructor
	// because hash calculation is called from blockchain initialization
	if err := fm.initializeSigners(); err != nil {
		return nil, err
	}

	return fm, nil
}

// Initialize initializes ForkManager on initialization phase
func (m *ForkManager) Initialize() error {
	if err := m.initializeValidatorStores(); err != nil {
		return err
	}

	return nil
}

// GetSigner returns a proper signer at specified height
func (m *ForkManager) GetSigner(height uint64) (signer.Signer, error) {
	fork := m.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	signer, ok := m.signers[fork.ValidatorType]
	if !ok {
		return nil, ErrSignerNotFound
	}

	return signer, nil
}

// GetValidatorStore returns a proper validator set at specified height
func (m *ForkManager) GetValidatorStore(height uint64) (ValidatorStore, error) {
	fork := m.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	set, ok := m.validatorSets[ibftTypesToSourceType[fork.Type]]
	if !ok {
		return nil, ErrValidatorStoreNotFound
	}

	return set, nil
}

// GetValidators returns validators at specified height
func (m *ForkManager) GetValidators(height uint64) (validators.Validators, error) {
	fork := m.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	set, err := m.GetValidatorStore(height)
	if err != nil {
		return nil, err
	}

	return set.GetValidators(
		height,
		m.epochSize,
		fork.From.Value,
	)
}

// GetHooks returns a hooks at specified height
func (m *ForkManager) GetHooks(height uint64) (*hook.Hooks, error) {
	hooks := &hook.Hooks{}

	fork := m.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	var err error

	switch fork.Type {
	case PoA:
		err = m.registerPoAHooks(hooks, height)
	case PoS:
		registerPoSHook(hooks, m.epochSize)
	}

	if err != nil {
		return nil, err
	}

	if err := m.registerPoAPrepareHooks(hooks, height); err != nil {
		return nil, err
	}

	m.registerPoSPrepareHooks(hooks, height)

	return hooks, nil
}

// Close calls termination process of submodules
func (m *ForkManager) Close() error {
	for _, store := range m.validatorSets {
		if err := store.Close(); err != nil {
			return err
		}
	}

	return nil
}

// initializeSigners initialize all signers based on Fork configuration
func (m *ForkManager) initializeSigners() error {
	for _, fork := range m.forks {
		valType := fork.ValidatorType

		if err := m.initializeSigner(valType); err != nil {
			return err
		}
	}

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

// initializeSigner initializes the specified signer
func (m *ForkManager) initializeSigner(valType validators.ValidatorType) error {
	if _, ok := m.signers[valType]; ok {
		return nil
	}

	signer, err := signer.NewSignerFromType(m.secretsManager, valType)
	if err != nil {
		return err
	}

	m.signers[valType] = signer

	return nil
}

// initializeValidatorStore initializes the specified validator set
func (m *ForkManager) initializeValidatorStore(setType store.SourceType) error {
	if _, ok := m.validatorSets[setType]; ok {
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

	m.validatorSets[setType] = valStore

	return nil
}

// registerPoAHooks register additional processes for PoA
func (m *ForkManager) registerPoAHooks(
	hooks *hook.Hooks,
	height uint64,
) error {
	valSet, err := m.GetValidatorStore(height)
	if err != nil {
		return err
	}

	registerValidatorStoreHook(hooks, valSet)

	return nil
}

// registerPoAHooks register additional processes to start PoA in the middle
func (m *ForkManager) registerPoAPrepareHooks(
	hooks *hook.Hooks,
	height uint64,
) error {
	fromFork := m.getForkByFrom(height + 1)
	if fromFork == nil || fromFork.Type != PoA || fromFork.Validators == nil {
		return nil
	}

	nextValSet, err := m.GetValidatorStore(height + 1)
	if err != nil {
		return err
	}

	registerUpdateValidatorStoreHook(
		hooks,
		nextValSet,
		fromFork.Validators,
		fromFork.From.Value,
	)

	return nil
}

// registerPoAHooks register additional processes to start PoS in the middle
func (m *ForkManager) registerPoSPrepareHooks(
	hooks *hook.Hooks,
	height uint64,
) {
	deploymentFork := m.getForkByDeployment(height + 1)
	if deploymentFork == nil || deploymentFork.Type != PoS || deploymentFork.Deployment == nil {
		return
	}

	registerContractDeploymentHook(hooks, deploymentFork)
}

// getFork returns a fork the specified height uses
func (m *ForkManager) getFork(height uint64) *IBFTFork {
	for idx := len(m.forks) - 1; idx >= 0; idx-- {
		fork := m.forks[idx]

		if fork.From.Value <= height && (fork.To == nil || height <= fork.To.Value) {
			return &fork
		}
	}

	return nil
}

// getForkByFrom returns a fork whose From matches with the specified height
func (m *ForkManager) getForkByFrom(height uint64) *IBFTFork {
	for _, fork := range m.forks {
		if fork.From.Value == height {
			return &fork
		}
	}

	return nil
}

// getForkByFrom returns a fork whose Development matches with the specified height
func (m *ForkManager) getForkByDeployment(height uint64) *IBFTFork {
	for _, fork := range m.forks {
		if fork.Deployment != nil && fork.Deployment.Value == height {
			return &fork
		}
	}

	return nil
}
