package tracker

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/tracker/store"
)

func setupDB(t *testing.T) (store.Store, func()) {
	t.Helper()

	dir, err := ioutil.TempDir("/tmp", "boltdb-test")
	require.NoError(t, err)

	path := filepath.Join(dir, "test.db")
	store, err := NewEventTrackerStore(path, 2, nil, hclog.Default())
	require.NoError(t, err)

	closeFn := func() {
		require.NoError(t, os.RemoveAll(dir))
	}

	return store, closeFn
}

func TestBoltDBStore(t *testing.T) {
	t.Parallel()

	store.TestStore(t, setupDB)
}
