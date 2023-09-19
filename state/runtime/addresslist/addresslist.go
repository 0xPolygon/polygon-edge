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
)

// list of gas costs for the operations
var (
	writeAddressListCost = uint64(20000)
	readAddressListCost  = uint64(5000)
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

	// all the functions have the same input (i.e. tuple(address)) which
	// in abi gets codified as a 32 bytes array with the first 20 bytes
	// encoding the address
	if len(inputBytes) != 32 {
		return nil, 0, errInputTooShort
	}

	var gasUsed uint64

	consumeGas := func(gasConsume uint64) error {
		if gas < gasConsume {
			return runtime.ErrOutOfGas
		}

		gasUsed = gasConsume

		return nil
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

	// Only Admin accounts can modify the role of other accounts
	addrRole := a.GetRole(caller)
	if addrRole != AdminRole {
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
