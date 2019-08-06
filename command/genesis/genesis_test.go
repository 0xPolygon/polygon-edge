package command

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func existsGenesis() bool {
	if _, err := os.Stat(genesisPath); err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	} else {
		panic("unexpected error")
	}
}

func TestGenesis(t *testing.T) {
	assert.False(t, existsGenesis())
	assert.NoError(t, genesisRunE(nil, nil))
	assert.True(t, existsGenesis())
	assert.EqualError(t, genesisRunE(nil, nil), "Genesis (./genesis.json) already exists")

	// clear and remove the genesis file
	if err := os.Remove(genesisPath); err != nil {
		t.Fatal(err)
	}
}
