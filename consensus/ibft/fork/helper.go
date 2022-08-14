package fork

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/0xPolygon/polygon-edge/validators/valset/snapshot"
	"github.com/hashicorp/go-hclog"
)

// removeFile is a helper function to remove a file
// log when error occurs
func removeFile(
	logger hclog.Logger,
	path string,
) {
	if err := os.Remove(path); err != nil {
		logger.Error("failed to remove file", "path", path, "err", err)
	}
}

// loadSnapshotMetadata loads Metadata from file
func loadSnapshotMetadata(
	logger hclog.Logger,
	path string,
) *snapshot.SnapshotMetadata {
	var meta *snapshot.SnapshotMetadata
	if err := readDataStore(path, &meta); err != nil {
		logger.Error("Could not read metadata snapshot store file", "err", err.Error())

		removeFile(logger, path)

		logger.Error("Removed invalid metadata snapshot store file")

		return nil
	}

	return meta
}

// loadSnapshots loads Snapshots from file
func loadSnapshots(
	logger hclog.Logger,
	path string,
) []*snapshot.Snapshot {
	snaps := []*snapshot.Snapshot{}
	if err := readDataStore(path, &snaps); err != nil {
		logger.Error("Could not read snapshot store file", "err", err.Error())

		removeFile(logger, path)

		logger.Error("Removed invalid snapshot store file")

		return nil
	}

	return snaps
}

// readDataStore attempts to read the specific file from file storage
func readDataStore(path string, obj interface{}) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, obj); err != nil {
		return err
	}

	return nil
}

// writeDataStore attempts to write the specific file to file storage
func writeDataStore(path string, obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	//nolint: gosec
	if err := ioutil.WriteFile(path, data, 0755); err != nil {
		return err
	}

	return nil
}
