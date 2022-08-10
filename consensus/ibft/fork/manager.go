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

type ForkManager interface {
	Initialize() error
	GetSigner(uint64) (signer.Signer, error)
	GetValidatorSet(uint64) (valset.ValidatorSet, error)
	GetValidators(uint64) (validators.Validators, error)
	GetHooks(uint64) (hook.Hooks, error)
	Close() error
}

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
	// because hash calculation is called on blockchain initialization
	if err := fm.initializeSigners(); err != nil {
		return nil, err
	}

	return fm, nil
}

func (m *forkManagerImpl) Initialize() error {
	if err := m.initializeValidatorSets(); err != nil {
		return err
	}

	return nil
}

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

func (m *forkManagerImpl) GetValidatorSet(height uint64) (valset.ValidatorSet, error) {
	fork := m.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	set, ok := m.validatorSets[ibftTypeToSourceType(fork.Type)]
	if !ok {
		return nil, ErrValidatorSetNotFound
	}

	return set, nil
}

func (m *forkManagerImpl) GetValidators(height uint64) (validators.Validators, error) {
	set, err := m.GetValidatorSet(height)
	if err != nil {
		return nil, err
	}

	return set.GetValidators(height)
}

func (m *forkManagerImpl) GetHooks(height uint64) (hook.Hooks, error) {
	hooks := &hook.HookManager{}

	fork := m.getFork(height)
	if fork == nil {
		return nil, ErrForkNotFound
	}

	if fork.Type == PoS {
		registerPoSHook(hooks, m.epochSize)
	}

	valSet, err := m.GetValidatorSet(height)
	if err != nil {
		return nil, err
	}

	registerValidatorSetHook(hooks, valSet)

	if deploymentFork := m.getForkByDeployment(height); deploymentFork != nil {
		registerContractDeploymentHook(hooks, deploymentFork)
	}

	return hooks, nil
}

func (m *forkManagerImpl) Close() error {
	if err := m.closeSnapshotValidatorSet(); err != nil {
		return err
	}

	return nil
}

func (m *forkManagerImpl) initializeSigners() error {
	for _, fork := range m.forks {
		valType := fork.ValidatorType

		if err := m.initializeSigner(valType); err != nil {
			return err
		}
	}

	return nil
}

func (m *forkManagerImpl) initializeValidatorSets() error {
	for _, fork := range m.forks {
		sourceType := ibftTypeToSourceType(fork.Type)
		if err := m.initializeValidatorSet(sourceType); err != nil {
			return err
		}
	}

	return nil
}

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
		valSet, err = m.initializeContractValidatorSet()
	}

	if err != nil {
		return err
	}

	m.validatorSets[setType] = valSet

	return nil
}

func (m *forkManagerImpl) initializeSnapshotValidatorSet() (valset.ValidatorSet, error) {
	snapshotMeta := loadSnapshotMetadata(m.logger, filepath.Join(m.filePath, snapshotMetadataFilename))
	snapshots := loadSnapshots(m.logger, filepath.Join(m.filePath, snapshotSnapshotsFilename))

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

func (m *forkManagerImpl) initializeContractValidatorSet() (valset.ValidatorSet, error) {
	return contract.NewContractValidatorSet(
		m.logger,
		m.blockchain,
		m.executor,
		m.GetSigner,
		m.epochSize,
	)
}

func (m *forkManagerImpl) getValidatorType(height uint64) (validators.ValidatorType, error) {
	fork := m.getFork(height)
	if fork == nil {
		return "", ErrForkNotFound
	}

	return fork.ValidatorType, nil
}

func (m *forkManagerImpl) getFork(height uint64) *IBFTFork {
	for idx := len(m.forks) - 1; idx >= 0; idx-- {
		fork := m.forks[idx]

		if fork.From.Value <= height && (fork.To == nil || height <= fork.To.Value) {
			return &fork
		}
	}

	return nil
}

func (m *forkManagerImpl) getForkByDeployment(height uint64) *IBFTFork {
	for _, fork := range m.forks {
		if fork.Deployment != nil && fork.Deployment.Value == height {
			return &fork
		}
	}

	return nil
}
