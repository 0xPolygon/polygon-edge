package addresslist

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

// list of function methods for the address list functionality
var (
	SetAdminFunc        = abi.MustNewMethod("function setAdmin(address)")
	SetEnabledFunc      = abi.MustNewMethod("function setEnabled(address)")
	SetNoneFunc         = abi.MustNewMethod("function setNone(address)")
	ReadAddressListFunc = abi.MustNewMethod("function readAddressList(address) returns (uint256)")
	SetListEnabledFunc  = abi.MustNewMethod("function setListEnabled(bool)")
	GetListEnabledFunc  = abi.MustNewMethod("function getListEnabled() returns (bool)")
)

// list of gas costs for the operations
var (
	writeAddressListCost = uint64(20000)
	readAddressListCost  = uint64(5000)
)

var (
	// is list enabled or not key hash
	enabledKeyHash = types.StringToHash("ffffffffffffffffffffffffffffffffffffffff")
	// super admin key hash
	superAdminKeyHash = types.StringToHash("fffffffffffffffffffffffffffffffffffffffe")
)

type AddressList struct {
	state stateRef
	addr  types.Address
}

func NewAddressList(state stateRef, addr types.Address) *AddressList {
	return &AddressList{state: state, addr: addr}
}

func (a *AddressList) Addr() types.Address {
	return a.addr
}

func (a *AddressList) Run(c *runtime.Contract, host runtime.Host, _ *chain.ForksInTime) *runtime.ExecutionResult {
	ret, gasUsed, err := a.runInputCall(c.Caller, c.Input, c.Gas, c.Static)

	res := &runtime.ExecutionResult{
		ReturnValue: ret,
		GasUsed:     gasUsed,
		GasLeft:     c.Gas - gasUsed,
		Err:         err,
	}

	return res
}

var (
	errNoFunctionSignature = fmt.Errorf("input is too short for a function call")
	errInputTooShort       = fmt.Errorf("wrong input size, expected 32")
	errFunctionNotFound    = fmt.Errorf("function not found")
	errWriteProtection     = fmt.Errorf("write protection")
	errAdminSelfRemove     = fmt.Errorf("cannot remove admin role from caller")
)

func (a *AddressList) runInputCall(caller types.Address, input []byte,
	gas uint64, isStatic bool) ([]byte, uint64, error) {
	// decode the function signature from the input
	if len(input) < types.SignatureSize {
		return nil, 0, errNoFunctionSignature
	}

	sig, inputBytes := input[:4], input[4:]

	var gasUsed uint64

	consumeGas := func(gasConsume uint64) error {
		if gas < gasConsume {
			return runtime.ErrOutOfGas
		}

		gasUsed = gasConsume

		return nil
	}

	// GetListEnabledFunc does not have any parameters and returns bool value
	if bytes.Equal(sig, GetListEnabledFunc.ID()) {
		if err := consumeGas(readAddressListCost); err != nil {
			return nil, 0, err
		}

		result := getAbiBoolValue(a.IsEnabled())

		return result, gasUsed, nil
	}

	// SetEnabledList receives bool as input parameter which in abi get codified
	// as a 32 bytes array
	// all the other functions have the same input (i.e. tuple(address)) which
	// in abi gets codified as a 32 bytes array with the first 20 bytes
	// encoding the address
	if len(inputBytes) != types.HashLength {
		return nil, 0, errInputTooShort
	}

	superAdmin, superAdminExists := a.GetSuperAdmin()
	isSuperAdmin := superAdminExists && superAdmin == caller

	if bytes.Equal(sig, SetListEnabledFunc.ID()) {
		if err := consumeGas(writeAddressListCost); err != nil {
			return nil, 0, err
		}

		if isSuperAdmin || a.GetRole(caller) == AdminRole {
			// any hash different than zero hash will be treated as true
			value := types.BytesToHash(input) != types.ZeroHash

			a.SetEnabled(value)

			return nil, gasUsed, nil
		}

		return nil, gasUsed, runtime.ErrNotAuth
	}

	inputAddr := types.BytesToAddress(inputBytes)

	if bytes.Equal(sig, ReadAddressListFunc.ID()) {
		if err := consumeGas(readAddressListCost); err != nil {
			return nil, 0, err
		}

		// read operation
		role := a.GetRole(inputAddr)

		return role.Bytes(), gasUsed, nil
	}

	// write operation
	var updateRole Role
	if bytes.Equal(sig, SetAdminFunc.ID()) {
		updateRole = AdminRole
	} else if bytes.Equal(sig, SetEnabledFunc.ID()) {
		updateRole = EnabledRole
	} else if bytes.Equal(sig, SetNoneFunc.ID()) {
		updateRole = NoRole
	} else {
		return nil, 0, errFunctionNotFound
	}

	if err := consumeGas(writeAddressListCost); err != nil {
		return nil, gasUsed, err
	}

	// we cannot perform any write operation if the call is static
	if isStatic {
		return nil, gasUsed, errWriteProtection
	}

	// Only Admin or superadmin accounts can modify the role of other accounts
	addrRole := a.GetRole(caller)
	if addrRole != AdminRole && !isSuperAdmin {
		return nil, gasUsed, runtime.ErrNotAuth
	}

	// An admin can not remove himself from the list
	if addrRole == AdminRole && caller == inputAddr {
		return nil, gasUsed, errAdminSelfRemove
	}

	a.SetRole(inputAddr, updateRole)

	return nil, gasUsed, nil
}

func (a *AddressList) SetRole(addr types.Address, role Role) {
	a.state.SetState(a.addr, types.BytesToHash(addr.Bytes()), types.Hash(role))
}

func (a *AddressList) GetRole(addr types.Address) Role {
	res := a.state.GetStorage(a.addr, types.BytesToHash(addr.Bytes()))

	return Role(res)
}

func (a *AddressList) IsEnabled() bool {
	return a.state.GetStorage(a.addr, enabledKeyHash) != types.ZeroHash
}

func (a *AddressList) SetEnabled(value bool) {
	stateValue := types.BytesToHash(getAbiBoolValue(value))

	a.state.SetState(a.addr, enabledKeyHash, stateValue)
}

func (a *AddressList) SetSuperAdmin(addr *types.Address) {
	if addr == nil {
		// if we want to clear superadmin, do not do anything if superadmin does not exists in storage
		if _, exists := a.GetSuperAdmin(); !exists {
			return
		}

		a.state.SetState(a.addr, superAdminKeyHash, types.ZeroHash)
	} else {
		value := types.BytesToHash(addr.Bytes())
		// The first byte specifies the presence of a super admin
		// (allowing the use of the types.ZeroAddress address for the superadmin)
		value[0] = 1

		a.state.SetState(a.addr, superAdminKeyHash, value)
	}
}

func (a *AddressList) GetSuperAdmin() (types.Address, bool) {
	res := a.state.GetStorage(a.addr, superAdminKeyHash)
	// If the first byte of the hash is zero, it indicates that no superadmin has been saved in the storage
	if res[0] == 0 {
		return types.ZeroAddress, false
	}

	return types.BytesToAddress(res[:]), true
}

type Role types.Hash

var (
	NoRole      Role = Role(types.StringToHash("0x0000000000000000000000000000000000000000000000000000000000000000"))
	EnabledRole Role = Role(types.StringToHash("0x0000000000000000000000000000000000000000000000000000000000000001"))
	AdminRole   Role = Role(types.StringToHash("0x0000000000000000000000000000000000000000000000000000000000000002"))
)

func (r Role) Uint64() uint64 {
	switch r {
	case EnabledRole:
		return 1
	case AdminRole:
		return 2
	default:
		return 0
	}
}

func (r Role) Bytes() []byte {
	return types.Hash(r).Bytes()
}

func (r Role) Enabled() bool {
	return r == AdminRole || r == EnabledRole
}

type stateRef interface {
	SetState(addr types.Address, key, value types.Hash)
	GetStorage(addr types.Address, key types.Hash) types.Hash
}

func getAbiBoolValue(value bool) []byte {
	encodedValue, _ := abi.MustNewType("bool").Encode(value)

	return encodedValue
}
