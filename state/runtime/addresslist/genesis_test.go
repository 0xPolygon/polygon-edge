package addresslist

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	t.Parallel()

	one := types.Address{0x1}
	two := types.Address{0x2}
	three := types.Address{0x3}
	owner := types.Address{0x88}

	// initial genesis chain with different types of
	// struct initialization
	gen := &chain.Genesis{
		Alloc: map[types.Address]*chain.GenesisAccount{},
	}

	config := &chain.AddressListConfig{
		AdminAddresses: []types.Address{
			one,
		},
		EnabledAddresses: []types.Address{
			two,
			three,
		},
	}

	// without owner and enabled

	ApplyGenesisAllocs(gen, types.Address{}, config, nil)

	expect := &chain.GenesisAccount{
		Balance: big.NewInt(1),
		Storage: map[types.Hash]types.Hash{
			types.BytesToHash(one.Bytes()):   types.Hash(AdminRole),
			types.BytesToHash(two.Bytes()):   types.Hash(EnabledRole),
			types.BytesToHash(three.Bytes()): types.Hash(EnabledRole),
		},
	}

	require.Equal(t, expect, gen.Alloc[types.Address{}])

	// with owner and disabled

	gen.Alloc = map[types.Address]*chain.GenesisAccount{}

	ApplyGenesisAllocs(gen, types.Address{}, nil, &owner)

	ownerHash := types.BytesToHash(owner.Bytes())
	ownerHash[0] = 1
	expect = &chain.GenesisAccount{
		Balance: big.NewInt(1),
		Storage: map[types.Hash]types.Hash{
			types.StringToHash("fffffffffffffffffffffffffffffffffffffffe"): ownerHash,
			types.StringToHash("ffffffffffffffffffffffffffffffffffffffff"): types.StringToHash("1"),
		},
	}

	require.Equal(t, expect, gen.Alloc[types.Address{}])
}
