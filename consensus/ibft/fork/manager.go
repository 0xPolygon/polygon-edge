package fork

import (
	"errors"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/hook"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/valset"
	"github.com/0xPolygon/polygon-edge/validators/valset/contract"
	"github.com/0xPolygon/polygon-edge/validators/valset/snapshot"
	"github.com/hashicorp/go-hclog"
)

const (
	loggerName                = "fork_manager"
	snapshotMetadataFilename  = "metadata"
	snapshotSnapshotsFilename = "snapshots"
)

var (
	ErrForkNotFound         = errors.New("fork not found")
	ErrSignerNotFound       = errors.New("signer not found")
	ErrValidatorSetNotFound = errors.New("validator set not found")
)

// ForkManager is an interface of the module that has Fork configuration and multiple version of submodules
// and returns the proper submodule at specified height
type ForkManager interface {
	Initialize() error
	GetSigner(uint64) (signer.Signer, error)
	GetValidatorSet(uint64) (valset.ValidatorSet, error)
	GetValidators(uint64) (validators.Validators, error)
	GetHooks(uint64) (hook.Hooks, error)
	Close() error
}

// forkManagerImpl is a implementation of ForkManager
type forkManagerImpl struct {
	logger         hclog.Logger
	blockchain     valset.HeaderGetter
	executor       contract.Executor
	secretsManager secrets.SecretsManager

	// configuration
	forks     []IBFTFork
	filePath  string
	epochSize uint64

	signers       map[validators.ValidatorType]signer.Signer
	validatorSets map[valset.SourceType]valset.ValidatorSet
}

// NewForkManager is a constructor of forkManagerImpl
func NewForkManager(
	logger hclog.Logger,
	blockchain valset.HeaderGetter,
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
		validatorSets:  make(map[valset.SourceType]valset.ValidatorSet),
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
	if err := m.initializeValidatorSets(); err != nil {
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

// GetValidatorSet returns a proper validator set at specified height
func (m *forkManagerImpl) GetValidatorSet(height uint64) (valset.ValidatorSet, error) {
	fork := m.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	set, ok := m.validatorSets[ibftTypesToSourceType[fork.Type]]
	if !ok {
		return nil, ErrValidatorSetNotFound
	}

	return set, nil
}

// GetValidatorSet returns validators at specified height
func (m *forkManagerImpl) GetValidators(height uint64) (validators.Validators, error) {
	set, err := m.GetValidatorSet(height)
	if err != nil {
		return nil, err
	}

	return set.GetValidators(height)
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
	if err := m.closeSnapshotValidatorSet(); err != nil {
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

// initializeValidatorSets initializes all validator sets based on Fork configuration
func (m *forkManagerImpl) initializeValidatorSets() error {
	for _, fork := range m.forks {
		sourceType := ibftTypesToSourceType[fork.Type]
		if err := m.initializeValidatorSet(sourceType); err != nil {
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

// initializeSigner initializes the specified validator set
func (m *forkManagerImpl) initializeValidatorSet(setType valset.SourceType) error {
	if _, ok := m.validatorSets[setType]; ok {
		return nil
	}

	var (
		valSet valset.ValidatorSet
		err    error
	)

	switch setType {
	case valset.Snapshot:
		valSet, err = m.initializeSnapshotValidatorSet()
	case valset.Contract:
		valSet = contract.NewContractValidatorSet(
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

// initializeSnapshotValidatorSet loads data from file and initializes Snapshot validator set
func (m *forkManagerImpl) initializeSnapshotValidatorSet() (valset.ValidatorSet, error) {
	snapshotMeta, err := loadSnapshotMetadata(filepath.Join(m.filePath, snapshotMetadataFilename))
	if err != nil {
		return nil, err
	}

	snapshots, err := loadSnapshots(filepath.Join(m.filePath, snapshotSnapshotsFilename))
	if err != nil {
		return nil, err
	}

	snapshotValset, err := snapshot.NewSnapshotValidatorSet(
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

// closeSnapshotValidatorSet gets data from Snapshot validator set and save to files
func (m *forkManagerImpl) closeSnapshotValidatorSet() error {
	snapshotValset, ok := m.validatorSets[valset.Snapshot].(*snapshot.SnapshotValidatorSet)
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
	valSet, err := m.GetValidatorSet(height)
	if err != nil {
		return err
	}

	registerValidatorSetHook(hooks, valSet)

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

	nextValSet, err := m.GetValidatorSet(height + 1)
	if err != nil {
		return err
	}

	registerUpdateValidatorSetHook(
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
