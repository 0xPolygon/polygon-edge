package addresslist

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/types"
)

func ApplyGenesisAllocs(
	chain *chain.Genesis,
	addressListAddr types.Address,
	config *chain.AddressListConfig,
	superAdmin *types.Address) {
	if superAdmin == nil && config == nil {
		return
	}

	allocList := &AddressList{
		addr:  addressListAddr,
		state: &genesisState{chain},
	}

	// if super admin is nil nothing will be written to the storage
	allocList.SetSuperAdmin(superAdmin)

	if config == nil {
		allocList.SetEnabled(false)
	} else {
		allocList.SetEnabled(true)

		// enabled addr
		for _, addr := range config.EnabledAddresses {
			allocList.SetRole(addr, EnabledRole)
		}

		// admin addr
		for _, addr := range config.AdminAddresses {
			allocList.SetRole(addr, AdminRole)
		}
	}
}

type genesisState struct {
	chain *chain.Genesis
}

func (g *genesisState) SetState(addr types.Address, key, value types.Hash) {
	alloc, ok := g.chain.Alloc[addr]
	if !ok {
		alloc = &chain.GenesisAccount{}
		g.chain.Alloc[addr] = alloc
	}

	// initialize a balance of at least 1 since otherwise
	// the evm understand that this account is empty
	alloc.Balance = big.NewInt(1)

	if alloc.Storage == nil {
		alloc.Storage = map[types.Hash]types.Hash{}
	}

	alloc.Storage[key] = value
}

func (g *genesisState) GetStorage(addr types.Address, key types.Hash) types.Hash {
	// since `genesisState` is used only as part of `ApplyGenesisAllocs` to set the initial
	// roles in the contract. It never calls this `GetStorage` function.
	return types.Hash{}
}
