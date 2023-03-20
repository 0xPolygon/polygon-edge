package allowlist

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

func newMockAllowList() *AllowList {
	state := &mockState{
		state: map[types.Hash]types.Hash{},
	}

	return NewAllowList(state, types.Address{})
}

func TestAllowList_WrongInput(t *testing.T) {
	a := newMockAllowList()

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

func TestAllowList_ReadOp_NotEnoughGas(t *testing.T) {
	a := newMockAllowList()

	input, _ := ReadAllowListFunc.Encode([]interface{}{types.Address{}})

	_, _, err := a.runInputCall(types.Address{}, input, 0, false)
	require.Equal(t, runtime.ErrOutOfGas, err)

	_, _, err = a.runInputCall(types.Address{}, input, readAllowListCost-1, false)
	require.Equal(t, runtime.ErrOutOfGas, err)
}

func TestAllowList_ReadOp_Full(t *testing.T) {
	a := newMockAllowList()
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
		input, _ := ReadAllowListFunc.Encode([]interface{}{c.addr})
		role, gasUsed, err := a.runInputCall(types.Address{}, input, readAllowListCost, false)
		require.NoError(t, err)
		require.Equal(t, gasUsed, readAllowListCost)
		require.Equal(t, c.role.Bytes(), role)
	}
}

func TestAllowList_WriteOp_NotEnoughGas(t *testing.T) {
	a := newMockAllowList()

	input, _ := SetAdminFunc.Encode([]interface{}{types.Address{}})

	_, _, err := a.runInputCall(types.Address{}, input, 0, false)
	require.Equal(t, runtime.ErrOutOfGas, err)

	_, _, err = a.runInputCall(types.Address{}, input, writeAllowListCost-1, false)
	require.Equal(t, runtime.ErrOutOfGas, err)
}

func TestAllowList_WriteOp_CannotWriteInStaticCall(t *testing.T) {
	a := newMockAllowList()

	input, _ := SetAdminFunc.Encode([]interface{}{types.Address{}})

	_, gasCost, err := a.runInputCall(types.Address{}, input, writeAllowListCost, true)
	require.Equal(t, writeAllowListCost, gasCost)
	require.Equal(t, err, errWriteProtection)
}

func TestAllowList_WriteOp_OnlyAdminCanUpdate(t *testing.T) {
	a := newMockAllowList()

	input, _ := SetAdminFunc.Encode([]interface{}{types.Address{}})

	_, gasCost, err := a.runInputCall(types.Address{}, input, writeAllowListCost, false)
	require.Equal(t, writeAllowListCost, gasCost)
	require.Equal(t, err, runtime.ErrNotAuth)
}

func TestAllowList_WriteOp_Full(t *testing.T) {
	a := newMockAllowList()
	a.SetRole(types.Address{}, AdminRole)

	targetAddr := types.Address{0x1}

	// ensure that the target account does not have a role so far
	require.Equal(t, NoRole, a.GetRole(targetAddr))

	cases := []struct {
		method *abi.Method
		role   Role
	}{
		{SetAdminFunc, AdminRole},
		{SetEnabledSignatureFunc, EnabledRole},
		{SetNoneFunc, NoRole},
	}

	for _, c := range cases {
		input, _ := c.method.Encode([]interface{}{targetAddr})

		ret, gasCost, err := a.runInputCall(types.Address{}, input, writeAllowListCost, false)
		require.Equal(t, writeAllowListCost, gasCost)
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
