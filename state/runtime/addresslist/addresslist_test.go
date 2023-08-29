package addresslist

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

var addressListsOwner = types.StringToAddress("ffffffffffffffffffffffffffffffffffffffff")

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

	al := NewAddressList(state, types.Address{})
	al.SetEnabled(true)
	al.SetOwner(&addressListsOwner)

	return al
}

func TestAddressList_WrongInput(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	a := newMockAddressList()

	input, _ := ReadAddressListFunc.Encode([]interface{}{types.Address{}})

	_, _, err := a.runInputCall(types.Address{}, input, 0, false)
	require.Equal(t, runtime.ErrOutOfGas, err)

	_, _, err = a.runInputCall(types.Address{}, input, readAddressListCost-1, false)
	require.Equal(t, runtime.ErrOutOfGas, err)
}

func TestAddressList_ReadOp_Full(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	a := newMockAddressList()

	input, _ := SetAdminFunc.Encode([]interface{}{types.Address{}})

	_, _, err := a.runInputCall(types.Address{}, input, 0, false)
	require.Equal(t, runtime.ErrOutOfGas, err)

	_, _, err = a.runInputCall(types.Address{}, input, writeAddressListCost-1, false)
	require.Equal(t, runtime.ErrOutOfGas, err)
}

func TestAddressList_WriteOp_CannotWriteInStaticCall(t *testing.T) {
	t.Parallel()

	a := newMockAddressList()

	input, _ := SetAdminFunc.Encode([]interface{}{types.Address{}})

	_, gasCost, err := a.runInputCall(types.Address{}, input, writeAddressListCost, true)
	require.Equal(t, writeAddressListCost, gasCost)
	require.Equal(t, err, errWriteProtection)
}

func TestAddressList_WriteOp_OnlyAdminCanUpdate(t *testing.T) {
	t.Parallel()

	a := newMockAddressList()

	input, _ := SetAdminFunc.Encode([]interface{}{types.Address{}})

	_, gasCost, err := a.runInputCall(types.Address{}, input, writeAddressListCost, false)
	require.Equal(t, writeAddressListCost, gasCost)
	require.Equal(t, err, runtime.ErrNotAuth)
}

func TestAddressList_WriteOp_Full(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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

func TestAddressList_GetRole_AddressListsOwner(t *testing.T) {
	t.Parallel()

	a := newMockAddressList()

	require.Equal(t, AdminRole, a.GetRole(addressListsOwner))
}

func TestAddressList_SetEnabled_IsStatic(t *testing.T) {
	t.Parallel()

	a := newMockAddressList()
	input, _ := SetListEnabledFunc.Encode([]interface{}{false})
	_, gasCost, err := a.runInputCall(addressListsOwner, input, writeAddressListCost, true)

	require.ErrorIs(t, err, errWriteProtection)
	require.Equal(t, writeAddressListCost, gasCost)
}

func TestAddressList_SetEnabled(t *testing.T) {
	t.Parallel()

	expectEnabled := func(t *testing.T, a *AddressList, caller types.Address, value bool) {
		t.Helper()

		input, _ := GetListEnabledFunc.Encode([]interface{}{})
		result, gasCost, err := a.runInputCall(caller, input, readAddressListCost, false)

		require.NoError(t, err)
		require.Equal(t, readAddressListCost, gasCost)

		resultVal, _ := abi.MustNewType("bool").Decode(result)

		require.Equal(t, value, resultVal)
	}

	someRndAddress := types.Address{0x99, 0xAA}
	admin := types.Address{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x99}
	a := newMockAddressList()

	a.SetRole(types.Address{}, EnabledRole)
	expectEnabled(t, a, someRndAddress, true)

	// only admin or owner should be able to disable list
	input, _ := SetListEnabledFunc.Encode([]interface{}{false})
	_, gasCost, err := a.runInputCall(types.ZeroAddress, input, writeAddressListCost, false)

	require.ErrorIs(t, err, runtime.ErrNotAuth)
	require.Equal(t, writeAddressListCost, gasCost)
	expectEnabled(t, a, someRndAddress, true)

	// disable list
	input, _ = SetListEnabledFunc.Encode([]interface{}{false})
	_, gasCost, err = a.runInputCall(addressListsOwner, input, writeAddressListCost, false)

	require.NoError(t, err)
	require.Equal(t, writeAddressListCost, gasCost)
	expectEnabled(t, a, admin, false)

	// owner can add admin role while list is disabled
	input, _ = SetAdminFunc.Encode([]interface{}{admin})
	_, gasCost, err = a.runInputCall(addressListsOwner, input, writeAddressListCost, false)

	require.NoError(t, err)
	require.Equal(t, writeAddressListCost, gasCost)
	require.Equal(t, AdminRole, a.GetRole(admin))

	// anyone can check role of newly added admin while list is disabled
	input, _ = ReadAddressListFunc.Encode([]interface{}{admin})
	role, gasCost, err := a.runInputCall(someRndAddress, input, writeAddressListCost, false)

	require.NoError(t, err)
	require.Equal(t, readAddressListCost, gasCost)
	require.Equal(t, AdminRole.Bytes(), role)

	// admin can add new role while list is disabled
	input, _ = SetEnabledFunc.Encode([]interface{}{someRndAddress})
	_, gasCost, err = a.runInputCall(admin, input, writeAddressListCost, false)

	require.NoError(t, err)
	require.Equal(t, writeAddressListCost, gasCost)
	require.Equal(t, EnabledRole, a.GetRole(someRndAddress))

	// non admin can not add role
	input, _ = SetEnabledFunc.Encode([]interface{}{types.Address{0xff, 0xfe}})
	_, gasCost, err = a.runInputCall(someRndAddress, input, writeAddressListCost, false)

	require.ErrorIs(t, err, runtime.ErrNotAuth)
	require.Equal(t, writeAddressListCost, gasCost)
	expectEnabled(t, a, admin, false)

	// admin can enable list too
	input, _ = SetListEnabledFunc.Encode([]interface{}{true})
	_, gasCost, err = a.runInputCall(admin, input, writeAddressListCost, false)

	require.NoError(t, err)
	require.Equal(t, writeAddressListCost, gasCost)
	expectEnabled(t, a, someRndAddress, true)
}

func TestAddressList_WriteOp_NonAdmin(t *testing.T) {
	t.Parallel()

	adminAddress := types.Address{0x95}
	enabledRoleAddress := types.Address{0x94}
	noRoleAddress := types.Address{0x93}
	randomAddress := types.Address{0x92}
	targetAddr := types.Address{0x1}

	a := newMockAddressList()
	a.SetRole(adminAddress, AdminRole)
	a.SetRole(enabledRoleAddress, EnabledRole)
	a.SetRole(noRoleAddress, NoRole)

	for _, callerAddress := range []types.Address{enabledRoleAddress, noRoleAddress, randomAddress} {
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

			_, gasCost, err := a.runInputCall(callerAddress, input, writeAddressListCost, false)
			require.Equal(t, writeAddressListCost, gasCost)
			require.ErrorIs(t, err, runtime.ErrNotAuth)
			require.Equal(t, NoRole, a.GetRole(targetAddr))
		}
	}
}
