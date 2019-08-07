package network

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBoltPeerStore(t *testing.T) {
	dir, err := ioutil.TempDir("", "example")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	p, err := NewBoltDBPeerStore(dir)
	assert.NoError(t, err)

	p.Update("1", 0)
	p.Update("2", 0)
	p.Update("3", 0)

	peers, err := p.Load()
	assert.NoError(t, err)
	assert.Equal(t, peers, []string{"1", "2", "3"})
}
