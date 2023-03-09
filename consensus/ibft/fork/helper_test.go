package fork

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	testHelper "github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/0xPolygon/polygon-edge/validators/store/snapshot"
	"github.com/stretchr/testify/assert"
)

var (
	sampleJSON = `{"Hello":"World"}`
	sampleMap  = map[string]interface{}{
		"Hello": "World",
	}
)

func createTestTempDirectory(t *testing.T) string {
	t.Helper()

	path, err := os.MkdirTemp("", "temp")
	if err != nil {
		t.Logf("failed to create temp directory, err=%+v", err)

		t.FailNow()
	}

	t.Cleanup(func() {
		os.RemoveAll(path)
	})

	return path
}

func Test_loadSnapshotMetadata(t *testing.T) {
	t.Parallel()

	t.Run("should return error if the file doesn't exist", func(t *testing.T) {
		t.Parallel()

		dirPath := createTestTempDirectory(t)
		filePath := path.Join(dirPath, "test.dat")

		res, err := loadSnapshotMetadata(filePath)

		assert.NoError(t, err)
		assert.Nil(t, res)
	})

	t.Run("should load metadata", func(t *testing.T) {
		t.Parallel()

		metadata := &snapshot.SnapshotMetadata{
			LastBlock: 100,
		}

		fileData, err := json.Marshal(metadata)
		assert.NoError(t, err)

		dirPath := createTestTempDirectory(t)
		filePath := path.Join(dirPath, "test.dat")
		assert.NoError(t, os.WriteFile(filePath, fileData, 0775))

		res, err := loadSnapshotMetadata(filePath)

		assert.NoError(
			t,
			err,
		)

		assert.Equal(
			t,
			metadata,
			res,
		)
	})
}

func Test_loadSnapshots(t *testing.T) {
	t.Parallel()

	t.Run("should return error if the file doesn't exist", func(t *testing.T) {
		t.Parallel()

		dirPath := createTestTempDirectory(t)
		filePath := path.Join(dirPath, "test.dat")

		res, err := loadSnapshots(filePath)

		assert.NoError(t, err)
		assert.Equal(t, []*snapshot.Snapshot{}, res)
	})

	t.Run("should load metadata", func(t *testing.T) {
		t.Parallel()

		snapshots := []*snapshot.Snapshot{
			{
				Number: 10,
				Hash:   types.BytesToHash(crypto.Keccak256([]byte{0x10})).String(),
				Set: validators.NewECDSAValidatorSet(
					validators.NewECDSAValidator(types.StringToAddress("1")),
				),
				Votes: []*store.Vote{
					{
						Candidate: validators.NewECDSAValidator(types.StringToAddress("2")),
						Validator: types.StringToAddress("1"),
						Authorize: true,
					},
				},
			},
		}

		fileData, err := json.Marshal(snapshots)
		assert.NoError(t, err)

		dirPath := createTestTempDirectory(t)
		filePath := path.Join(dirPath, "test.dat")
		assert.NoError(t, os.WriteFile(filePath, fileData, 0775))

		res, err := loadSnapshots(filePath)

		assert.NoError(
			t,
			err,
		)

		assert.Equal(
			t,
			snapshots,
			res,
		)
	})
}

func Test_readDataStore(t *testing.T) {
	t.Parallel()

	t.Run("should return error if the file doesn't exist", func(t *testing.T) {
		t.Parallel()

		dirPath := createTestTempDirectory(t)
		filePath := path.Join(dirPath, "test.dat")
		var data interface{}

		assert.Equal(
			t,
			nil,
			readDataStore(filePath, data),
		)
	})

	t.Run("should return error if the content is not json", func(t *testing.T) {
		t.Parallel()

		dirPath := createTestTempDirectory(t)
		filePath := path.Join(dirPath, "test.dat")

		assert.NoError(
			t,
			os.WriteFile(filePath, []byte("hello: world"), 0775),
		)

		data := map[string]interface{}{}

		assert.IsType(
			t,
			&json.SyntaxError{},
			readDataStore(filePath, data),
		)
	})

	t.Run("should read and map to object", func(t *testing.T) {
		t.Parallel()

		dirPath := createTestTempDirectory(t)
		filePath := path.Join(dirPath, "test.dat")

		assert.NoError(
			t,
			os.WriteFile(filePath, []byte(sampleJSON), 0775),
		)

		data := map[string]interface{}{}

		assert.NoError(
			t,
			readDataStore(filePath, &data),
		)

		assert.Equal(
			t,
			sampleMap,
			data,
		)
	})
}

func Test_writeDataStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		data               interface{}
		expectedStoredData string
		expectedErr        error
	}{
		{
			name:               "should return error if json.Marshal failed",
			data:               func() {},
			expectedStoredData: "",
			expectedErr:        errors.New("json: unsupported type: func()"),
		},
		{
			name: "should return error if WriteFile failed",
			data: map[string]interface{}{
				"Hello": "World",
			},
			expectedStoredData: sampleJSON,
			expectedErr:        nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dirPath := createTestTempDirectory(t)
			filePath := path.Join(dirPath, "test.dat")

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				writeDataStore(filePath, test.data),
			)

			readData, _ := os.ReadFile(filePath)

			assert.Equal(
				t,
				test.expectedStoredData,
				string(readData),
			)
		})
	}
}
