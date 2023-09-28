package runtime

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

var (
	address1 = types.BytesToAddress([]byte("address1"))
	address2 = types.BytesToAddress([]byte("address2"))
	address3 = types.BytesToAddress([]byte("address3"))
	slotHash = types.BytesToHash([]byte("slot"))
)

func createInitialAccessList() *AccessList {
	initialAccessList := NewAccessList()
	(*initialAccessList)[address1] = make(map[types.Hash]struct{})
	(*initialAccessList)[address1][slotHash] = struct{}{}
	(*initialAccessList)[address2] = make(map[types.Hash]struct{})

	return initialAccessList
}

func TestContainsAddress(t *testing.T) {
	t.Parallel()

	initialAccessList := createInitialAccessList()
	assert.True(t, initialAccessList.ContainsAddress(address1))
	assert.False(t, initialAccessList.ContainsAddress(address3))
}

func TestContains(t *testing.T) {
	t.Parallel()

	initialAccessList := createInitialAccessList()

	address1Present, slotHashPresent := initialAccessList.Contains(address1, slotHash)
	assert.True(t, address1Present)
	assert.True(t, slotHashPresent)

	address3Present, slotHashPresent := initialAccessList.Contains(address3, slotHash)
	assert.False(t, address3Present)
	assert.False(t, slotHashPresent)
}

func TestCopy(t *testing.T) {
	t.Parallel()

	initialAccessList := createInitialAccessList()
	copiedAccessList := initialAccessList.Copy()
	assert.Equal(t, initialAccessList, copiedAccessList)
}

func TestAddAddress(t *testing.T) {
	t.Parallel()

	initialAccessList := createInitialAccessList()
	finalAccessList := createInitialAccessList()

	initialAccessList.AddAddress(address1)
	assert.Equal(t, finalAccessList, initialAccessList)

	initialAccessList.AddAddress(address3)

	(*finalAccessList)[address3] = make(map[types.Hash]struct{})
	assert.Equal(t, finalAccessList, initialAccessList)
}

func TestAddSlot(t *testing.T) {
	t.Parallel()

	initialAccessList := createInitialAccessList()
	finalAccessList := createInitialAccessList()

	// add address1 and slotHash
	addr1Exists, slot1Exists := initialAccessList.AddSlot(address1, slotHash)
	assert.False(t, addr1Exists)
	assert.False(t, slot1Exists)
	assert.Equal(t, finalAccessList, initialAccessList)

	// add address2 and slotHash
	addr2Exists, slot2Exists := initialAccessList.AddSlot(address2, slotHash)
	assert.False(t, addr2Exists)
	assert.True(t, slot2Exists)

	(*finalAccessList)[address2][slotHash] = struct{}{}
	assert.Equal(t, finalAccessList, initialAccessList)

	// add address3 and slotHash
	addr3Exists, slot3Exists := initialAccessList.AddSlot(address3, slotHash)
	assert.True(t, addr3Exists)
	assert.True(t, slot3Exists)

	(*finalAccessList)[address3] = make(map[types.Hash]struct{})
	(*finalAccessList)[address3][slotHash] = struct{}{}
	assert.Equal(t, finalAccessList, initialAccessList)
}
