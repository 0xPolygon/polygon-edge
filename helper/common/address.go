package common

import "github.com/0xPolygon/polygon-edge/types"

// DiffAddresses returns a list of addresses that are in setA but not in setB
func DiffAddresses(setA, setB []types.Address) []types.Address {
	existedInAddr2 := make(map[types.Address]bool)
	for _, addr := range setB {
		existedInAddr2[addr] = true
	}

	diff := make([]types.Address, 0)

	for _, addr := range setA {
		if !existedInAddr2[addr] {
			diff = append(diff, addr)
		}
	}

	return diff
}
