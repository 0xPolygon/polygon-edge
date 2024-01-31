package runtime

import (
	"github.com/0xPolygon/polygon-edge/types"
)

type AccessList map[types.Address]map[types.Hash]struct{}

func NewAccessList() *AccessList {
	al := make(AccessList)

	return &al
}

// Checks if  address is present within the access list.
func (al *AccessList) ContainsAddress(address types.Address) bool {
	_, ok := (*al)[address]

	return ok
}

// Contains checks if a slot is present in an account.
// Returns two boolean flags: `accountPresent` and `slotPresent`.
func (al *AccessList) Contains(address types.Address, slot types.Hash) (bool, bool) {
	_, addrPresent := (*al)[address]
	_, slotPresent := (*al)[address][slot]

	return addrPresent, slotPresent
}

// Copy creates an deep copy of provided AccessList.
func (al *AccessList) Copy() *AccessList {
	cp := make(AccessList, len(*al))

	for addr, slotMap := range *al {
		cpSlotMap := make(map[types.Hash]struct{}, len(slotMap))
		for slotHash := range slotMap {
			cpSlotMap[slotHash] = struct{}{}
		}

		cp[addr] = cpSlotMap
	}

	return &cp
}

// AddAddress adds an address to the access list
// returns 'true' if the operation results in a change (i.e., the address was not already present in the list).
func (al *AccessList) AddAddress(address ...types.Address) {
	for _, addr := range address {
		if _, exists := (*al)[addr]; exists {
			continue
		}

		(*al)[addr] = make(map[types.Hash]struct{})
	}
}

// This function adds the specified address and slot pair to the access list.
// The return values indicate whether the address was newly added and whether the slot was newly added.
func (al *AccessList) AddSlot(address types.Address, slot types.Hash) (addrChange bool, slotChange bool) {
	slotMap, addressExists := (*al)[address]
	if !addressExists {
		slotMap = make(map[types.Hash]struct{})
		(*al)[address] = slotMap
	}

	_, slotPresent := slotMap[slot]
	if !slotPresent {
		slotMap[slot] = struct{}{}

		return !addressExists, true
	}

	// slot and address were already present in access list
	return false, false
}
