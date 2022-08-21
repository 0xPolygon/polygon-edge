package fork

import (
	"encoding/json"
	"io/ioutil"
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
