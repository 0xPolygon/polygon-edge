package fork

import (
	"encoding/json"
	"os"

	"github.com/0xPolygon/polygon-edge/validators/store/snapshot"
)

// loadSnapshotMetadata loads Metadata from file
func loadSnapshotMetadata(path string) (*snapshot.SnapshotMetadata, error) {
	var meta *snapshot.SnapshotMetadata
	if err := readDataStore(path, &meta); err != nil {
		return nil, err
	}

	return meta, nil
}

// loadSnapshots loads Snapshots from file
func loadSnapshots(path string) ([]*snapshot.Snapshot, error) {
	snaps := []*snapshot.Snapshot{}
	if err := readDataStore(path, &snaps); err != nil {
		return nil, err
	}

	return snaps, nil
}

// readDataStore attempts to read the specific file from file storage
// return nil if the file doesn't exist
func readDataStore(path string, obj interface{}) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(path)
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

	if err := os.WriteFile(path, data, os.ModePerm); err != nil {
		return err
	}

	return nil
}
