package filter

import (
	"testing"

	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func TestFilterManager(t *testing.T) {

}

func TestTimeout(t *testing.T) {
	// TODO
}

func TestHeadStream(t *testing.T) {
	b := &blockStream{}

	b.push(types.StringToHash("1"))
	b.push(types.StringToHash("2"))

	cur := b.Head()

	b.push(types.StringToHash("3"))
	b.push(types.StringToHash("4"))

	// get the updates, there are two new entries
	updates, next := cur.getUpdates()

	assert.Equal(t, updates[0], types.StringToHash("3"))
	assert.Equal(t, updates[1], types.StringToHash("4"))

	// there are no new entries
	updates, _ = next.getUpdates()
	assert.Len(t, updates, 0)
}
