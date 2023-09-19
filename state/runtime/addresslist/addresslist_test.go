package addresslist

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

type mockState struct {
	state map[types.Hash]types.Hash
}

func (m *mockState) SetState(addr types.Address, key, value types.Hash) {
	m.state[key] = value
}

func (m *mockState) GetStorage(addr types.Address, key types.Hash) types.Hash {
	return m.state[key]
}

func newMockAddressList() *AddressList {
	state := &mockState{
		state: map[types.Hash]types.Hash{},
	}

	return NewAddressList(state, types.Address{})
}

func TestAddressList_WrongInput(t *testing.T) {
	a := newMockAddressList()

	input := []byte{}

	// no function signature
	_, _, err := a.runInputCall(types.Address{}, input, 0, false)
	require.Equal(t, errNoFunctionSignature, err)

	input = append(input, []byte{0x1, 0x2, 0x3, 0x4}...)

	// no function input
	_, _, err = a.runInputCall(types.Address{}, input, 0, false)
	require.Equal(t, errInputTooShort, err)

	input = append(input, make([]byte, 32)...)

	// wrong signature
	_, _, err = a.runInputCall(types.Address{}, input, 0, false)
	require.Equal(t, errFunctionNotFound, err)
}

func TestAddressList_ReadOp_NotEnoughGas(t *testing.T) {
	a := newMockAddressList()

	input, _ := ReadAddressListFunc.Encode([]interface{}{types.Address{}})

	_, _, err := a.runInputCall(types.Address{}, input, 0, false)
	require.Equal(t, runtime.ErrOutOfGas, err)

	_, _, err = a.runInputCall(types.Address{}, input, readAddressListCost-1, false)
	require.Equal(t, runtime.ErrOutOfGas, err)
}

func TestAddressList_ReadOp_Full(t *testing.T) {
	a := newMockAddressList()
	a.SetRole(types.Address{}, AdminRole)

	cases := []struct {
		addr types.Address
		role Role
	}{
		{
			// return the role for an existing address
			types.Address{},
			AdminRole,
		},
		{
			// return the role for a non-existing address
			types.Address{0x1},
			NoRole,
		},
	}

	for _, c := range cases {
		input, _ := ReadAddressListFunc.Encode([]interface{}{c.addr})
		role, gasUsed, err := a.runInputCall(types.Address{}, input, readAddressListCost, false)
		require.NoError(t, err)
		require.Equal(t, gasUsed, readAddressListCost)
		require.Equal(t, c.role.Bytes(), role)
	}
}

func TestAddressList_WriteOp_NotEnoughGas(t *testing.T) {
	a := newMockAddressList()

	input, _ := SetAdminFunc.Encode([]interface{}{types.Address{}})

	_, _, err := a.runInputCall(types.Address{}, input, 0, false)
	require.Equal(t, runtime.ErrOutOfGas, err)

	_, _, err = a.runInputCall(types.Address{}, input, writeAddressListCost-1, false)
	require.Equal(t, runtime.ErrOutOfGas, err)
}

func TestAddressList_WriteOp_CannotWriteInStaticCall(t *testing.T) {
	a := newMockAddressList()

	input, _ := SetAdminFunc.Encode([]interface{}{types.Address{}})

	_, gasCost, err := a.runInputCall(types.Address{}, input, writeAddressListCost, true)
	require.Equal(t, writeAddressListCost, gasCost)
	require.Equal(t, err, errWriteProtection)
}

func TestAddressList_WriteOp_OnlyAdminCanUpdate(t *testing.T) {
	a := newMockAddressList()

	input, _ := SetAdminFunc.Encode([]interface{}{types.Address{}})

	_, gasCost, err := a.runInputCall(types.Address{}, input, writeAddressListCost, false)
	require.Equal(t, writeAddressListCost, gasCost)
	require.Equal(t, err, runtime.ErrNotAuth)
}

func TestAddressList_WriteOp_Full(t *testing.T) {
	a := newMockAddressList()
	a.SetRole(types.Address{}, AdminRole)

	targetAddr := types.Address{0x1}

	// ensure that the target account does not have a role so far
	require.Equal(t, NoRole, a.GetRole(targetAddr))

	cases := []struct {
		method *abi.Method
		role   Role
	}{
		{SetAdminFunc, AdminRole},
		{SetEnabledFunc, EnabledRole},
		{SetNoneFunc, NoRole},
	}

	for _, c := range cases {
		input, _ := c.method.Encode([]interface{}{targetAddr})

		ret, gasCost, err := a.runInputCall(types.Address{}, input, writeAddressListCost, false)
		require.Equal(t, writeAddressListCost, gasCost)
		require.NoError(t, err)
		require.Empty(t, ret)
		require.Equal(t, c.role, a.GetRole(targetAddr))
	}
}

func TestRole_ToUint(t *testing.T) {
	cases := []struct {
		role Role
		num  uint64
	}{
		{AdminRole, uint64(2)},
		{EnabledRole, uint64(1)},
		{NoRole, uint64(0)},
	}

	for _, c := range cases {
		require.Equal(t, c.num, c.role.Uint64())
	}
}

func TestRole_Enabled(t *testing.T) {
	cases := []struct {
		role    Role
		enabled bool
	}{
		{AdminRole, true},
		{EnabledRole, true},
		{NoRole, false},
	}

	for _, c := range cases {
		require.Equal(t, c.enabled, c.role.Enabled())
	}
}
