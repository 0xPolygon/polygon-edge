package fork

import (
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/0xPolygon/polygon-edge/validators/store/contract"
	"github.com/0xPolygon/polygon-edge/validators/store/snapshot"
	"github.com/hashicorp/go-hclog"
)

// Closer is an interface for termination process
type Closer interface {
	Close() error
}

type ValidatorsGetter interface {
	GetValidators(height, epochSize, forkFrom uint64) (validators.Validators, error)
}

// SnapshotValidatorStoreWrapper is a wrapper of store.SnapshotValidatorStore
// in order to add initialization and closer process with side effect
type SnapshotValidatorStoreWrapper struct {
	*snapshot.SnapshotValidatorStore
	dirPath string
}

// Close saves SnapshotValidator data into local storage
func (w *SnapshotValidatorStoreWrapper) Close() error {
	// save data
	var (
		metadata  = w.GetSnapshotMetadata()
		snapshots = w.GetSnapshots()
	)

	if err := writeDataStore(filepath.Join(w.dirPath, snapshotMetadataFilename), metadata); err != nil {
		return err
	}

	if err := writeDataStore(filepath.Join(w.dirPath, snapshotSnapshotsFilename), snapshots); err != nil {
		return err
	}

	return nil
}

// GetValidators returns validators at the specific height
func (w *SnapshotValidatorStoreWrapper) GetValidators(height, _, _ uint64) (validators.Validators, error) {
	// the biggest height of blocks that have been processed before the given height
	return w.GetValidatorsByHeight(height - 1)
}

// NewSnapshotValidatorStoreWrapper loads data from local storage and creates *SnapshotValidatorStoreWrapper
func NewSnapshotValidatorStoreWrapper(
	logger hclog.Logger,
	blockchain store.HeaderGetter,
	getSigner store.SignerGetter,
	dirPath string,
	epochSize uint64,
) (*SnapshotValidatorStoreWrapper, error) {
	snapshotMeta, err := loadSnapshotMetadata(filepath.Join(dirPath, snapshotMetadataFilename))
	if err != nil {
		return nil, err
	}

	snapshots, err := loadSnapshots(filepath.Join(dirPath, snapshotSnapshotsFilename))
	if err != nil {
		return nil, err
	}

	snapshotStore, err := snapshot.NewSnapshotValidatorStore(
		logger,
		blockchain,
		getSigner,
		epochSize,
		snapshotMeta,
		snapshots,
	)

	if err != nil {
		return nil, err
	}

	return &SnapshotValidatorStoreWrapper{
		SnapshotValidatorStore: snapshotStore,
		dirPath:                dirPath,
	}, nil
}

// ContractValidatorStoreWrapper is a wrapper of *contract.ContractValidatorStore
// in order to add Close and GetValidators
type ContractValidatorStoreWrapper struct {
	*contract.ContractValidatorStore
}

// NewContractValidatorStoreWrapper creates *ContractValidatorStoreWrapper
func NewContractValidatorStoreWrapper(
	logger hclog.Logger,
	blockchain store.HeaderGetter,
	executor contract.Executor,
	getSigner store.SignerGetter,
) (*ContractValidatorStoreWrapper, error) {
	contractStore, err := contract.NewContractValidatorStore(
		logger,
		blockchain,
		executor,
		getSigner,
		contract.DefaultValidatorSetCacheSize,
	)

	if err != nil {
		return nil, err
	}

	return &ContractValidatorStoreWrapper{
		contractStore,
	}, nil
}

// Close is closer process
func (w *ContractValidatorStoreWrapper) Close() error {
	return nil
}

// GetValidators gets and returns validators at the given height
func (w *ContractValidatorStoreWrapper) GetValidators(
	height, epochSize, forkFrom uint64,
) (validators.Validators, error) {
	return w.GetValidatorsByHeight(
		calculateContractStoreFetchingHeight(
			height,
			epochSize,
			forkFrom,
		),
	)
}

// calculateContractStoreFetchingHeight calculates the block height at which ContractStore fetches validators
// based on height, epoch, and fork beginning height
func calculateContractStoreFetchingHeight(height, epochSize, forkFrom uint64) uint64 {
	// calculates the beginning of the epoch the given height is in
	beginningEpoch := (height / epochSize) * epochSize

	// calculates the end of the previous epoch
	// to determine the height to fetch validators
	fetchingHeight := uint64(0)
	if beginningEpoch > 0 {
		fetchingHeight = beginningEpoch - 1
	}

	// use the calculated height if it's bigger than or equal to from
	if fetchingHeight >= forkFrom {
		return fetchingHeight
	}

	if forkFrom > 0 {
		return forkFrom - 1
	}

	return forkFrom
}
