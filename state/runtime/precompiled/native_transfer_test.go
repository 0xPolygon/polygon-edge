package precompiled

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

func Test_NativeTransferPrecompile(t *testing.T) {
	var (
		sender   = types.Address{0x1}
		receiver = types.Address{0x2}
	)

	// Create an instance of the state
	st := itrie.NewState(itrie.NewMemoryStorage())
	// Create a snapshot
	snapshot := st.NewSnapshot()
	// Create a radix
	radix := state.NewTxn(st, snapshot)
	transition := state.NewTransition(chain.AllForksEnabled.At(0), radix)
	contract := &nativeTransfer{}
	abiType := abi.MustNewType("tuple(address, address, uint256)")
	run := func(caller, from, to types.Address, amount *big.Int, host runtime.Host) error {
		input, err := abiType.Encode([]interface{}{from, to, amount})
		require.NoError(t, err)

		_, err = contract.run(input, caller, host)

		return err
	}

	t.Run("Invalid input", func(t *testing.T) {
		_, err := contract.run([]byte{}, types.Address{}, transition)
		require.ErrorIs(t, err, runtime.ErrInvalidInputData)
	})
	t.Run("Caller not authorized", func(t *testing.T) {
		err := run(types.ZeroAddress, sender, receiver, big.NewInt(10), transition)
		require.ErrorIs(t, err, runtime.ErrUnauthorizedCaller)
	})
	t.Run("Insufficient balance", func(t *testing.T) {
		err := run(contracts.NativeTokenContract, sender, receiver, big.NewInt(10), transition)
		require.ErrorIs(t, err, runtime.ErrInsufficientBalance)
	})
	t.Run("Correct transfer", func(t *testing.T) {
		stateTrie := state.NewTxn(st, snapshot)
		stateTrie.CreateAccount(sender)
		stateTrie.CreateAccount(receiver)
		stateTrie.AddBalance(sender, big.NewInt(1000))
		transition := state.NewTransition(chain.AllForksEnabled.At(0), stateTrie)

		require.NoError(t, run(contracts.NativeTokenContract, sender, receiver, big.NewInt(100), transition))
		require.Equal(t, big.NewInt(900), stateTrie.GetBalance(sender))
		require.Equal(t, big.NewInt(100), stateTrie.GetBalance(receiver))
	})
}
