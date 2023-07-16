package runtime

import (
	"github.com/0xPolygon/polygon-edge/types"
)

type AccessList struct {
	addresses map[types.Address]int
	slots     []map[types.Hash]struct{}
}

// ContainsAddress returns true if the address is in the access list.
func (al *AccessList) ContainsAddress(address types.Address) bool {
	_, ok := al.addresses[address]
	return ok
}

// Contains checks if a slot within an account is present in the access list, returning
// separate flags for the presence of the account and the slot respectively.
func (al *AccessList) Contains(address types.Address, slot types.Hash) (addressPresent bool, slotPresent bool) {
	idx, ok := al.addresses[address]
	if !ok {
		// no such address (and hence zero slots)
		return false, false
	}
	if idx == -1 {
		// address yes, but no slots
		return true, false
	}
	_, slotPresent = al.slots[idx][slot]
	return true, slotPresent
}

// newAccessList creates a new AccessList.
func NewAccessList() *AccessList {
	return &AccessList{
		addresses: make(map[types.Address]int),
	}
}

// Copy creates an independent copy of an AccessList.
func (a *AccessList) Copy() *AccessList {
	cp := NewAccessList()
	for k, v := range a.addresses {
		cp.addresses[k] = v
	}
	cp.slots = make([]map[types.Hash]struct{}, len(a.slots))
	for i, slotMap := range a.slots {
		newSlotmap := make(map[types.Hash]struct{}, len(slotMap))
		for k := range slotMap {
			newSlotmap[k] = struct{}{}
		}
		cp.slots[i] = newSlotmap
	}

	return cp
}

// AddAddress adds an address to the access list, and returns 'true' if the operation
// caused a change (addr was not previously in the list).
func (al *AccessList) AddAddress(address types.Address) bool {
	if _, present := al.addresses[address]; present {
		return false
	}
	al.addresses[address] = -1

	return true
}

// AddSlot adds the specified (addr, slot) combo to the access list.
// Return values are:
// - address added
// - slot added
func (al *AccessList) AddSlot(address types.Address, slot types.Hash) (addrChange bool, slotChange bool) {
	idx, addressPresent := al.addresses[address]
	if !addressPresent || idx == -1 {
		// Address not present, or addr present but no slots there
		al.addresses[address] = len(al.slots)
		slotmap := map[types.Hash]struct{}{slot: {}}
		al.slots = append(al.slots, slotmap)

		return !addressPresent, true
	}

	// There is already an (address,slot) mapping
	slotmap := al.slots[idx]
	if _, ok := slotmap[slot]; !ok {
		slotmap[slot] = struct{}{}

		return false, true
	}

	// No changes required
	return false, false
}
