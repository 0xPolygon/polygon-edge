package runtime

import (
	"github.com/0xPolygon/polygon-edge/types"
)

type JournalRevision struct {
	ID    int
	Index int
}

type JournalEntry interface {
	Revert(host Host)
}

type Journal struct {
	entries []JournalEntry
}

func (j *Journal) Append(entry JournalEntry) {
	j.entries = append(j.entries, entry)
}

func (j *Journal) Revert(host Host, snapshot int) {
	for i := len(j.entries) - 1; i >= snapshot; i-- {
		// Undo the changes made by the operation
		j.entries[i].Revert(host)
	}

	j.entries = j.entries[:snapshot]
}

func (j *Journal) Len() int {
	return len(j.entries)
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

var _ JournalEntry = (*AccessListAddAccountChange)(nil)

func (ch AccessListAddAccountChange) Revert(host Host) {
	host.DeleteAccessListAddress(ch.Address)
}

var _ JournalEntry = (*AccessListAddSlotChange)(nil)

func (ch AccessListAddSlotChange) Revert(host Host) {
	host.DeleteAccessListSlot(ch.Address, ch.Slot)
}
