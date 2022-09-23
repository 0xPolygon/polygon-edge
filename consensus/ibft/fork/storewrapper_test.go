package fork

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	testHelper "github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/0xPolygon/polygon-edge/validators/store/snapshot"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

var (
	errTest = errors.New("test")
)

func createTestMetadataJSON(height uint64) string {
	return fmt.Sprintf(`{"LastBlock": %d}`, height)
}

func createTestSnapshotJSON(t *testing.T, snapshot *snapshot.Snapshot) string {
	t.Helper()

	res, err := json.Marshal(snapshot)
	assert.NoError(t, err)

	return string(res)
}

func TestSnapshotValidatorStoreWrapper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		storedSnapshotMetadata string
		storedSnapshots        string
		blockchain             store.HeaderGetter
		signer                 signer.Signer
		epochSize              uint64
		err                    error
	}{
		{
			name:                   "should return error if loading metadata fails",
			storedSnapshotMetadata: `hoge`,
			storedSnapshots:        "",
			blockchain:             nil,
			signer:                 nil,
			epochSize:              0,
			err:                    &json.SyntaxError{},
		},
		{
			name:                   "should return error if loading snapshots fails",
			storedSnapshotMetadata: createTestMetadataJSON(10),
			storedSnapshots:        `fuga`,
			blockchain:             nil,
			signer:                 nil,
			epochSize:              0,
			err:                    &json.SyntaxError{},
		},
		{
			name:                   "should return error if initialize fails",
			storedSnapshotMetadata: createTestMetadataJSON(0),
			storedSnapshots:        "[]",
			blockchain: &store.MockBlockchain{
				HeaderFn: func() *types.Header {
					return &types.Header{Number: 0}
				},
			},
			signer:    nil,
			epochSize: 10,
			err:       fmt.Errorf("signer not found %d", 0),
		},
		{
			name:                   "should succeed",
			storedSnapshotMetadata: createTestMetadataJSON(10),
			storedSnapshots: fmt.Sprintf("[%s]", createTestSnapshotJSON(
				t,
				&snapshot.Snapshot{
					Number: 10,
					Hash:   types.BytesToHash([]byte{0x10}).String(),
					Set:    validators.NewECDSAValidatorSet(),
					Votes:  []*store.Vote{},
				},
			)),
			blockchain: &store.MockBlockchain{
				HeaderFn: func() *types.Header {
					return &types.Header{Number: 10}
				},
			},
			signer:    nil,
			epochSize: 10,
			err:       nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dirPath := createTestTempDirectory(t)

			assert.NoError(
				t,
				os.WriteFile(path.Join(dirPath, snapshotMetadataFilename), []byte(test.storedSnapshotMetadata), os.ModePerm),
			)

			assert.NoError(
				t,
				os.WriteFile(path.Join(dirPath, snapshotSnapshotsFilename), []byte(test.storedSnapshots), os.ModePerm),
			)

			store, err := NewSnapshotValidatorStoreWrapper(
				hclog.NewNullLogger(),
				test.blockchain,
				func(u uint64) (signer.Signer, error) {
					return test.signer, nil
				},
				dirPath,
				test.epochSize,
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.err,
				err,
			)

			if store != nil {
				assert.Equal(
					t,
					dirPath,
					store.dirPath,
				)
			}
		})
	}
}

func TestSnapshotValidatorStoreWrapperGetValidators(t *testing.T) {
	t.Parallel()

	var (
		epochSize uint64 = 10
		metadata         = &snapshot.SnapshotMetadata{
			LastBlock: 10,
		}
		snapshots = []*snapshot.Snapshot{
			{
				Number: 10,
				Hash:   types.StringToHash("1").String(),
				Set: validators.NewECDSAValidatorSet(
					validators.NewECDSAValidator(types.StringToAddress("1")),
				),
				Votes: []*store.Vote{},
			},
		}
	)

	store, err := snapshot.NewSnapshotValidatorStore(
		hclog.NewNullLogger(),
		&store.MockBlockchain{
			HeaderFn: func() *types.Header {
				return &types.Header{Number: 10}
			},
		},
		func(u uint64) (snapshot.SignerInterface, error) {
			return nil, nil
		},
		epochSize,
		metadata,
		snapshots,
	)

	assert.NoError(t, err)

	wrapper := SnapshotValidatorStoreWrapper{
		SnapshotValidatorStore: store,
	}

	vals, err := wrapper.GetValidators(11, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, snapshots[0].Set, vals)
}

func TestSnapshotValidatorStoreWrapperClose(t *testing.T) {
	t.Parallel()

	var (
		dirPath = createTestTempDirectory(t)

		epochSize uint64 = 10
		metadata         = &snapshot.SnapshotMetadata{
			LastBlock: 10,
		}
		snapshots = []*snapshot.Snapshot{
			{
				Number: 10,
				Hash:   types.StringToHash("1").String(),
				Set: validators.NewECDSAValidatorSet(
					validators.NewECDSAValidator(types.StringToAddress("1")),
				),
				Votes: []*store.Vote{},
			},
		}
	)

	store, err := snapshot.NewSnapshotValidatorStore(
		hclog.NewNullLogger(),
		&store.MockBlockchain{
			HeaderFn: func() *types.Header {
				return &types.Header{Number: 10}
			},
		},
		func(u uint64) (snapshot.SignerInterface, error) {
			return nil, nil
		},
		epochSize,
		metadata,
		snapshots,
	)

	assert.NoError(t, err)

	wrapper := SnapshotValidatorStoreWrapper{
		dirPath:                dirPath,
		SnapshotValidatorStore: store,
	}

	assert.NoError(t, wrapper.Close())

	savedMetadataFile, err := os.ReadFile(path.Join(dirPath, snapshotMetadataFilename))
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		createTestMetadataJSON(metadata.LastBlock),
		string(savedMetadataFile),
	)

	savedSnapshots, err := os.ReadFile(path.Join(dirPath, snapshotSnapshotsFilename))
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		fmt.Sprintf("[%s]", createTestSnapshotJSON(t, snapshots[0])),
		string(savedSnapshots),
	)
}

type MockExecutor struct {
	BeginTxnFunc func(types.Hash, *types.Header, types.Address) (*state.Transition, error)
}

func (m *MockExecutor) BeginTxn(hash types.Hash, header *types.Header, addr types.Address) (*state.Transition, error) {
	return m.BeginTxnFunc(hash, header, addr)
}

func TestNewContractValidatorStoreWrapper(t *testing.T) {
	t.Parallel()

	_, err := NewContractValidatorStoreWrapper(
		hclog.NewNullLogger(),
		&store.MockBlockchain{},
		&MockExecutor{},
		func(u uint64) (signer.Signer, error) {
			return nil, nil
		},
	)

	assert.NoError(t, err)
}

func TestNewContractValidatorStoreWrapperClose(t *testing.T) {
	t.Parallel()

	wrapper, err := NewContractValidatorStoreWrapper(
		hclog.NewNullLogger(),
		&store.MockBlockchain{},
		&MockExecutor{},
		func(u uint64) (signer.Signer, error) {
			return nil, nil
		},
	)

	assert.NoError(t, err)
	assert.NoError(t, wrapper.Close())
}

func TestNewContractValidatorStoreWrapperGetValidators(t *testing.T) {
	t.Parallel()

	t.Run("should return error if getSigner returns error", func(t *testing.T) {
		t.Parallel()

		wrapper, err := NewContractValidatorStoreWrapper(
			hclog.NewNullLogger(),
			&store.MockBlockchain{},
			&MockExecutor{},
			func(u uint64) (signer.Signer, error) {
				return nil, errTest
			},
		)

		assert.NoError(t, err)

		res, err := wrapper.GetValidators(0, 0, 0)
		assert.Nil(t, res)
		assert.ErrorIs(t, errTest, err)
	})

	t.Run("should return error if GetValidatorsByHeight returns error", func(t *testing.T) {
		t.Parallel()

		wrapper, err := NewContractValidatorStoreWrapper(
			hclog.NewNullLogger(),
			&store.MockBlockchain{
				GetHeaderByNumberFn: func(u uint64) (*types.Header, bool) {
					return nil, false
				},
			},
			&MockExecutor{},
			func(u uint64) (signer.Signer, error) {
				return signer.NewSigner(
					&signer.ECDSAKeyManager{},
					nil,
				), nil
			},
		)

		assert.NoError(t, err)

		res, err := wrapper.GetValidators(10, 10, 0)
		assert.Nil(t, res)
		assert.ErrorContains(t, err, "header not found at 9")
	})
}

func Test_calculateContractStoreFetchingHeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		height    uint64
		epochSize uint64
		forkFrom  uint64
		expected  uint64
	}{
		{
			name:      "should return 0 if the height is 2 (in the first epoch)",
			height:    2,
			epochSize: 10,
			forkFrom:  0,
			expected:  0,
		},
		{
			name:      "should return 0 if the height is 9 (in the first epoch)",
			height:    9,
			epochSize: 10,
			forkFrom:  0,
			expected:  0,
		},
		{
			name:      "should return 9 if the height is 10 (in the second epoch)",
			height:    10,
			epochSize: 10,
			forkFrom:  0,
			expected:  9,
		},
		{
			name:      "should return 9 if the height is 19 (in the second epoch)",
			height:    19,
			epochSize: 10,
			forkFrom:  0,
			expected:  9,
		},
		{
			name:      "should return 49 if the height is 10 but forkFrom is 50",
			height:    10,
			epochSize: 10,
			forkFrom:  50,
			expected:  49,
		},
		{
			name:      "should return 59 if the height is 60 and forkFrom is 50",
			height:    60,
			epochSize: 10,
			forkFrom:  50,
			expected:  59,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				calculateContractStoreFetchingHeight(test.height, test.epochSize, test.forkFrom),
			)
		})
	}
}
