package fork

import (
	"errors"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/hook"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/0xPolygon/polygon-edge/validators/store/contract"
	"github.com/0xPolygon/polygon-edge/validators/store/snapshot"
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

// ForkManager is an interface of the module that has Fork configuration and multiple version of submodules
// and returns the proper submodule at specified height
type ForkManager interface {
	Initialize() error
	GetSigner(uint64) (signer.Signer, error)
	GetValidatorStore(uint64) (store.ValidatorStore, error)
	GetValidators(uint64) (validators.Validators, error)
	GetHooks(uint64) (hook.Hooks, error)
	Close() error
}

// forkManagerImpl is a implementation of ForkManager
type forkManagerImpl struct {
	logger         hclog.Logger
	blockchain     store.HeaderGetter
	executor       contract.Executor
	secretsManager secrets.SecretsManager

	// configuration
	forks     []IBFTFork
	filePath  string
	epochSize uint64

	signers       map[validators.ValidatorType]signer.Signer
	validatorSets map[store.SourceType]store.ValidatorStore
}

// NewForkManager is a constructor of forkManagerImpl
func NewForkManager(
	logger hclog.Logger,
	blockchain store.HeaderGetter,
	executor contract.Executor,
	secretManager secrets.SecretsManager,
	filePath string,
	epochSize uint64,
	ibftConfig map[string]interface{},
) (ForkManager, error) {
	forks, err := GetIBFTForks(ibftConfig)
	if err != nil {
		return nil, err
	}

	fm := &forkManagerImpl{
		logger:         logger.Named(loggerName),
		blockchain:     blockchain,
		executor:       executor,
		secretsManager: secretManager,
		filePath:       filePath,
		epochSize:      epochSize,
		forks:          forks,
		signers:        make(map[validators.ValidatorType]signer.Signer),
		validatorSets:  make(map[store.SourceType]store.ValidatorStore),
	}

	// Need initialization of signers in the constructor
	// because hash calculation is called from blockchain initialization
	if err := fm.initializeSigners(); err != nil {
		return nil, err
	}

	return fm, nil
}

// Initialize initializes ForkManager on initialization phase
func (m *forkManagerImpl) Initialize() error {
	if err := m.initializeValidatorStores(); err != nil {
		return err
	}

	return nil
}

// GetSigner returns a proper signer at specified height
func (m *forkManagerImpl) GetSigner(height uint64) (signer.Signer, error) {
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
func (m *forkManagerImpl) GetValidatorStore(height uint64) (store.ValidatorStore, error) {
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
func (m *forkManagerImpl) GetValidators(height uint64) (validators.Validators, error) {
	fork := m.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	set, err := m.GetValidatorStore(height)
	if err != nil {
		return nil, err
	}

	return set.GetValidators(
		calculateFetchingValidatorsHeight(height, m.epochSize, fork),
	)
}

// GetHooks returns a hooks at specified height
func (m *forkManagerImpl) GetHooks(height uint64) (hook.Hooks, error) {
	hooks := &hook.HookManager{}

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
func (m *forkManagerImpl) Close() error {
	if err := m.closeSnapshotValidatorStore(); err != nil {
		return err
	}

	return nil
}

// initializeSigners initialize all signers based on Fork configuration
func (m *forkManagerImpl) initializeSigners() error {
	for _, fork := range m.forks {
		valType := fork.ValidatorType

		if err := m.initializeSigner(valType); err != nil {
			return err
		}
	}

	return nil
}

// initializeValidatorStores initializes all validator sets based on Fork configuration
func (m *forkManagerImpl) initializeValidatorStores() error {
	for _, fork := range m.forks {
		sourceType := ibftTypesToSourceType[fork.Type]
		if err := m.initializeValidatorStore(sourceType); err != nil {
			return err
		}
	}

	return nil
}

// initializeSigner initializes the specified signer
func (m *forkManagerImpl) initializeSigner(valType validators.ValidatorType) error {
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
func (m *forkManagerImpl) initializeValidatorStore(setType store.SourceType) error {
	if _, ok := m.validatorSets[setType]; ok {
		return nil
	}

	var (
		valSet store.ValidatorStore
		err    error
	)

	switch setType {
	case store.Snapshot:
		valSet, err = m.initializeSnapshotValidatorStore()
	case store.Contract:
		valSet = contract.NewContractValidatorStore(
			m.logger,
			m.blockchain,
			m.executor,
			m.GetSigner,
			m.epochSize,
		)
	}

	if err != nil {
		return err
	}

	m.validatorSets[setType] = valSet

	return nil
}

// initializeSnapshotValidatorStore loads data from file and initializes Snapshot validator set
func (m *forkManagerImpl) initializeSnapshotValidatorStore() (store.ValidatorStore, error) {
	snapshotMeta, err := loadSnapshotMetadata(filepath.Join(m.filePath, snapshotMetadataFilename))
	if err != nil {
		return nil, err
	}

	snapshots, err := loadSnapshots(filepath.Join(m.filePath, snapshotSnapshotsFilename))
	if err != nil {
		return nil, err
	}

	snapshotValset, err := snapshot.NewSnapshotValidatorStore(
		m.logger,
		m.blockchain,
		m.GetSigner,
		m.epochSize,
		snapshotMeta,
		snapshots,
	)

	if err != nil {
		return nil, err
	}

	return snapshotValset, nil
}

// closeSnapshotValidatorStore gets data from Snapshot validator set and save to files
func (m *forkManagerImpl) closeSnapshotValidatorStore() error {
	snapshotValset, ok := m.validatorSets[store.Snapshot].(*snapshot.SnapshotValidatorStore)
	if !ok {
		// no snapshot validator set, skip
		return nil
	}

	// save data
	var (
		metadata  = snapshotValset.GetSnapshotMetadata()
		snapshots = snapshotValset.GetSnapshots()
	)

	if err := writeDataStore(filepath.Join(m.filePath, "metadata"), metadata); err != nil {
		return err
	}

	if err := writeDataStore(filepath.Join(m.filePath, "snapshots"), snapshots); err != nil {
		return err
	}

	return nil
}

// registerPoAHooks register additional processes for PoA
func (m *forkManagerImpl) registerPoAHooks(
	hooks *hook.HookManager,
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
func (m *forkManagerImpl) registerPoAPrepareHooks(
	hooks *hook.HookManager,
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
func (m *forkManagerImpl) registerPoSPrepareHooks(
	hooks *hook.HookManager,
	height uint64,
) {
	deploymentFork := m.getForkByDeployment(height + 1)
	if deploymentFork == nil || deploymentFork.Type != PoS || deploymentFork.Deployment == nil {
		return
	}

	registerContractDeploymentHook(hooks, deploymentFork)
}

// getFork returns a fork the specified height uses
func (m *forkManagerImpl) getFork(height uint64) *IBFTFork {
	for idx := len(m.forks) - 1; idx >= 0; idx-- {
		fork := m.forks[idx]

		if fork.From.Value <= height && (fork.To == nil || height <= fork.To.Value) {
			return &fork
		}
	}

	return nil
}

// getForkByFrom returns a fork whose From matches with the specified height
func (m *forkManagerImpl) getForkByFrom(height uint64) *IBFTFork {
	for _, fork := range m.forks {
		if fork.From.Value == height {
			return &fork
		}
	}

	return nil
}

// getForkByFrom returns a fork whose Development matches with the specified height
func (m *forkManagerImpl) getForkByDeployment(height uint64) *IBFTFork {
	for _, fork := range m.forks {
		if fork.Deployment != nil && fork.Deployment.Value == height {
			return &fork
		}
	}

	return nil
}

func calculateFetchingValidatorsHeight(height, epochSize uint64, fork *IBFTFork) uint64 {
	switch ibftTypesToSourceType[fork.Type] {
	case store.Snapshot:
		// the biggest height of blocks that have been processed before the given height
		return height - 1
	case store.Contract:
		// calculates the beginning of the epoch the given height is in
		beginningEpoch := (height / epochSize) * epochSize

		// calculates the end of the previous epoch
		// to determine the height to fetch validators
		fetchingHeight := uint64(0)
		if beginningEpoch > 0 {
			fetchingHeight = beginningEpoch - 1
		}

		from := fork.From.Value

		// use the calculated height if it's bigger than or equal to from
		if fetchingHeight >= from {
			return fetchingHeight
		}

		if from > 0 {
			return from - 1
		}

		return from
	}

	return 0
}
