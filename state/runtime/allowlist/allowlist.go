package allowlist

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

// TODO: This is a placeholder, to be decided
var (
	AllowListContractsAddr = types.StringToAddress("0x0200000000000000000000000000000000000000")
)

// list of function methods for the allow list functionality
var (
	SetAdminFunc            = abi.MustNewMethod("function setAdmin(address)")
	SetEnabledSignatureFunc = abi.MustNewMethod("function setEnabled(address)")
	SetNoneFunc             = abi.MustNewMethod("function setNone(address)")
	ReadAllowListFunc       = abi.MustNewMethod("function readAllowList(address) returns (uint256)")
)

// list of gas costs
var (
	writeAllowListCost = uint64(20000)
	readAllowListCost  = uint64(5000)
)

type AllowList struct {
	state stateRef
	addr  types.Address
}

func NewAllowList(state stateRef, addr types.Address) *AllowList {
	return &AllowList{state: state, addr: addr}
}

func (a *AllowList) Addr() types.Address {
	return a.addr
}

func (a *AllowList) Run(c *runtime.Contract, host runtime.Host, _ *chain.ForksInTime) *runtime.ExecutionResult {
	ret, gasUsed, err := a.runInputCall(c.Caller, c.Input, c.Gas, c.Static)

	fmt.Println("- err == -", err)

	res := &runtime.ExecutionResult{
		ReturnValue: ret,
		GasUsed:     gasUsed,
		GasLeft:     c.Gas - gasUsed,
		Err:         err,
	}

	return res
}

func (a *AllowList) runInputCall(caller types.Address, input []byte, gas uint64, isStatic bool) ([]byte, uint64, error) {

	fmt.Println("-- input --", caller, input, gas, isStatic)

	// decode the function signature from the input
	if len(input) < 4 {
		return nil, 0, fmt.Errorf("input is too short for a function call")
	}

	sig, inputBytes := input[:4], input[4:]

	// all the functions have the same input (i.e. tuple(address)) which
	// in abi gets codified as a 32 bytes array with the first 20 bytes
	// encoding the address
	if len(inputBytes) != 32 {
		return nil, 0, fmt.Errorf("wrong input size, expected 32 but found %d", len(inputBytes))
	}

	var gasUsed uint64
	consumeGas := func(gasConsume uint64) error {
		if gas < gasConsume {
			return fmt.Errorf("out of gas")
		}
		gasUsed = gasConsume
		return nil
	}

	inputAddr := types.BytesToAddress(inputBytes)

	fmt.Println("A1")
	if bytes.Equal(sig, ReadAllowListFunc.ID()) {
		fmt.Println("=> READ ALLOW LIST! <=")

		if err := consumeGas(readAllowListCost); err != nil {
			return nil, 0, err
		}

		// read operation
		role := a.GetRole(inputAddr)

		fmt.Println("- role --", a.addr, inputAddr, role)

		return role.Bytes(), 0, nil
	}

	fmt.Println("A2")
	// write operation
	var updateRole Role
	if bytes.Equal(sig, SetAdminFunc.ID()) {
		updateRole = AdminRole
	} else if bytes.Equal(sig, SetEnabledSignatureFunc.ID()) {
		updateRole = EnabledRole
	} else if bytes.Equal(sig, SetNoneFunc.ID()) {
		updateRole = NoRole
	} else {
		return nil, gasUsed, fmt.Errorf("function not found")
	}

	fmt.Println("A3")
	if err := consumeGas(writeAllowListCost); err != nil {
		return nil, gasUsed, err
	}

	fmt.Println("A4")
	// we cannot perform any write operation if the call is static
	if isStatic {
		return nil, gasUsed, fmt.Errorf("write protection for static calls")
	}

	fmt.Println("A5")
	// Only Admin accounts can modify the role of other accounts
	addrRole := a.GetRole(caller)
	if addrRole != AdminRole {
		return nil, gasUsed, fmt.Errorf("not an admin")
	}

	fmt.Println("_ SET ROLE _", a.addr, inputAddr, updateRole)

	a.SetRole(inputAddr, updateRole)

	fmt.Println(a.GetRole(inputAddr))

	return nil, gasUsed, nil
}

func (a *AllowList) SetRole(addr types.Address, role Role) {
	a.state.SetState(a.addr, types.BytesToHash(addr.Bytes()), types.Hash(role))
}

func (a *AllowList) GetRole(addr types.Address) Role {
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
	if r == EnabledRole {
		return 1
	}
	if r == AdminRole {
		return 2
	}
	return 0
}

func (r Role) Bytes() []byte {
	return types.Hash(r).Bytes()
}

// TODO: Test
func (r Role) Enabled() bool {
	return r == AdminRole || r == EnabledRole
}

type stateRef interface {
	SetState(addr types.Address, key, value types.Hash)
	GetStorage(addr types.Address, key types.Hash) types.Hash
}
