package precompiled

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/contracts"
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

	contract := &nativeTransfer{}
	abiType := abi.MustNewType("tuple(address, address, uint256)")
	run := func(caller, from, to types.Address, amount *big.Int, host runtime.Host) error {
		input, err := abiType.Encode([]interface{}{from, to, amount})
		require.NoError(t, err)

		_, err = contract.run(input, caller, host)

		return err
	}

	t.Run("Invalid input", func(t *testing.T) {
		_, err := contract.run([]byte{}, types.Address{}, nil)
		require.ErrorIs(t, err, runtime.ErrInvalidInputData)
	})
	t.Run("Caller not authorized", func(t *testing.T) {
		err := run(types.ZeroAddress, sender, receiver, big.NewInt(10), nil)
		require.ErrorIs(t, err, runtime.ErrUnauthorizedCaller)
	})
	t.Run("Insufficient balance", func(t *testing.T) {
		err := run(contracts.NativeERC20TokenContract, sender, receiver, big.NewInt(10), newDummyHost(t))
		require.ErrorIs(t, err, runtime.ErrInsufficientBalance)
	})
	t.Run("Correct transfer", func(t *testing.T) {
		host := newDummyHost(t)
		host.AddBalance(sender, big.NewInt(1000))

		err := run(contracts.NativeERC20TokenContract, sender, receiver, big.NewInt(100), host)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(900), host.GetBalance(sender))
		require.Equal(t, big.NewInt(100), host.GetBalance(receiver))
	})
}

// d dummyHost
var _ runtime.Host = (*dummyHost)(nil)

type dummyHost struct {
	t *testing.T

	balances map[types.Address]*big.Int
}

func newDummyHost(t *testing.T) *dummyHost {
	t.Helper()

	return &dummyHost{
		t:        t,
		balances: map[types.Address]*big.Int{},
	}
}

func (d dummyHost) AddBalance(addr types.Address, balance *big.Int) {
	existingBalance := d.GetBalance(addr)
	existingBalance = new(big.Int).Add(existingBalance, balance)
	d.balances[addr] = existingBalance
}

func (d dummyHost) AccountExists(addr types.Address) bool {
	d.t.Fatalf("AccountExists is not implemented")

	return false
}

func (d dummyHost) GetStorage(addr types.Address, key types.Hash) types.Hash {
	d.t.Fatalf("GetStorage is not implemented")

	return types.ZeroHash
}

func (d dummyHost) SetState(
	addr types.Address,
	key types.Hash,
	value types.Hash,
) {
	d.t.Fatalf("SetState is not implemented")
}

func (d dummyHost) SetStorage(addr types.Address, key types.Hash, value types.Hash, config *chain.ForksInTime) runtime.StorageStatus {
	d.t.Fatalf("SetStorage is not implemented")

	return runtime.StorageAdded
}

func (d dummyHost) SetNonPayable(nonPayable bool) {
	d.t.Fatalf("SetNonPayable is not implemented")
}

func (d dummyHost) GetBalance(addr types.Address) *big.Int {
	balance, exists := d.balances[addr]
	if !exists {
		return big.NewInt(0)
	}

	return balance
}

func (d dummyHost) GetCodeSize(addr types.Address) int {
	d.t.Fatalf("GetCodeSize is not implemented")

	return -1
}

func (d dummyHost) GetCodeHash(addr types.Address) types.Hash {
	d.t.Fatalf("GetCodeHash is not implemented")

	return types.ZeroHash
}

func (d dummyHost) GetCode(addr types.Address) []byte {
	d.t.Fatalf("GetCode is not implemented")

	return nil
}

func (d dummyHost) Selfdestruct(addr types.Address, beneficiary types.Address) {
	d.t.Fatalf("Selfdestruct is not implemented")
}

func (d dummyHost) GetTxContext() runtime.TxContext {
	d.t.Fatalf("GetTxContext is not implemented")

	return runtime.TxContext{}
}

func (d dummyHost) GetBlockHash(number int64) types.Hash {
	d.t.Fatalf("GetTxContext is not implemented")

	return types.ZeroHash
}

func (d dummyHost) EmitLog(addr types.Address, topics []types.Hash, data []byte) {
	d.t.Fatalf("EmitLog is not implemented")
}

func (d dummyHost) Callx(_ *runtime.Contract, _ runtime.Host) *runtime.ExecutionResult {
	d.t.Fatalf("Callx is not implemented")

	return nil
}

func (d dummyHost) Empty(addr types.Address) bool {
	d.t.Fatalf("Callx is not implemented")

	return true
}

func (d dummyHost) GetNonce(addr types.Address) uint64 {
	d.t.Fatalf("GetNonce is not implemented")

	return 0
}

func (d dummyHost) Transfer(from types.Address, to types.Address, amount *big.Int) error {
	if d.balances == nil {
		d.balances = map[types.Address]*big.Int{}
	}

	senderBalance := d.GetBalance(from)
	if senderBalance.Cmp(amount) < 0 {
		return runtime.ErrInsufficientBalance
	}

	senderBalance = new(big.Int).Sub(senderBalance, amount)
	d.balances[from] = senderBalance

	receiverBalance := d.GetBalance(to)
	d.balances[to] = new(big.Int).Add(receiverBalance, amount)

	return nil
}

func (d dummyHost) GetTracer() runtime.VMTracer {
	return nil
}

func (d dummyHost) GetRefund() uint64 {
	return 0
}
