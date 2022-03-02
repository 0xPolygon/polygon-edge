package common

import "github.com/0xPolygon/polygon-edge/types"

// DiffAddresses returns a list of addresses that are in arr1 but not in arr2
func DiffAddresses(addrs1, addrs2 []types.Address) []types.Address {
	existedInAddr2 := make(map[types.Address]bool)
	for _, addr := range addrs2 {
		existedInAddr2[addr] = true
	}

	diff := make([]types.Address, 0)

	for _, addr := range addrs1 {
		if !existedInAddr2[addr] {
			diff = append(diff, addr)
		}
	}

	return diff
}
