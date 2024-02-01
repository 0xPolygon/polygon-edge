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
	var (
		addrPresent, slotPresent bool
		slots                    map[types.Hash]struct{}
	)

	slots, addrPresent = (*al)[address]
	if addrPresent {
		_, slotPresent = slots[slot]
	}

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
func (al *AccessList) AddAddress(address ...types.Address) {
	for _, addr := range address {
		if _, exists := (*al)[addr]; exists {
			continue
		}

		(*al)[addr] = make(map[types.Hash]struct{})
	}
}

// This function adds the specified address and slot pairs to the access list
func (al *AccessList) AddSlot(address types.Address, slot ...types.Hash) {
	slotMap, addressExists := (*al)[address]
	if !addressExists {
		slotMap = make(map[types.Hash]struct{})
		(*al)[address] = slotMap
	}

	for _, s := range slot {
		_, slotPresent := slotMap[s]
		if !slotPresent {
			slotMap[s] = struct{}{}
		}
	}
}

// PrepareAccessList prepares the access list for a transaction by adding addresses and storage slots.
// The precompiled contract addresses are added to the access list as well.
func (al *AccessList) PrepareAccessList(
	from types.Address,
	to *types.Address,
	precompiles []types.Address,
	txAccessList types.TxAccessList) {
	al.AddAddress(from)

	if to != nil {
		al.AddAddress(*to)
	}

	// add the precompiles
	al.AddAddress(precompiles...)

	// add accessList provided with access list and dynamic tx
	for _, accessListTuple := range txAccessList {
		al.AddAddress(accessListTuple.Address)
		al.AddSlot(accessListTuple.Address, accessListTuple.StorageKeys...)
	}
}

// DeleteAddress deletes the specified address from the AccessList.
func (al *AccessList) DeleteAddress(address types.Address) {
	delete(*al, address)
}

// DeleteSlot deletes the specified slot from the access list for the given address.
// If the address is not found in the access list, the method does nothing.
func (al *AccessList) DeleteSlot(address types.Address, slot types.Hash) {
	if slotMap, ok := (*al)[address]; ok {
		delete(slotMap, slot)
	}
}
