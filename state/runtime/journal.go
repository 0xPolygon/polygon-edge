package runtime

import (
	"github.com/0xPolygon/polygon-edge/types"
)

type JournalEntry interface {
	revert(c *Contract)
}

type Journal struct {
	entries []JournalEntry
}

func (j *Journal) Append(entry JournalEntry) {
	j.entries = append(j.entries, entry)
}

func (j *Journal) Revert(c *Contract) {
	for i := len(j.entries) - 1; i >= 0; i-- {
		j.entries[i].revert(c)
	}

	j.entries = j.entries[:0]
}

type (
	AccessListAddAccountChange struct {
		Address types.Address
	}
	AccessListAddSlotChange struct {
		Address types.Address
		Slot    types.Hash
	}
)

func (ch AccessListAddAccountChange) revert(c *Contract) {
	c.AccessList.DeleteAddress(ch.Address)
}

func (ch AccessListAddSlotChange) revert(c *Contract) {
	c.AccessList.DeleteSlot(ch.Address, ch.Slot)
}
