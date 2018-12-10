package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/umbracle/minimal/evm"
)

var (
	errInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
)

type stateCase struct {
	Info        *info                `json:"_info"`
	Env         *env                 `json:"env"`
	Pre         stateSnapshop        `json:"pre"`
	Post        map[string]postState `json:"post"`
	Transaction *stTransaction       `json:"transaction"`
}

// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
func IntrinsicGas(data []byte, contractCreation, homestead bool) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if contractCreation && homestead {
		gas = params.TxGasContractCreation
	} else {
		gas = params.TxGas
	}
	// Bump the required gas by the amount of transactional data
	if len(data) > 0 {
		// Zero and non-zero bytes are priced differently
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		// Make sure we don't exceed uint64 for all data combinations
		if (math.MaxUint64-gas)/params.TxDataNonZeroGas < nz {
			return 0, vm.ErrOutOfGas
		}
		gas += nz * params.TxDataNonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			return 0, vm.ErrOutOfGas
		}
		gas += z * params.TxDataZeroGas
	}
	return gas, nil
}

// Based on geth state_transition.go
type Transition struct {
	state      *state.StateDB
	env        *evm.Env
	config     *params.ChainConfig
	gas        uint64
	initialGas uint64
	msg        *types.Message
	gp         *core.GasPool
}

func (t *Transition) useGas(amount uint64) error {
	if t.gas < amount {
		return vm.ErrOutOfGas
	}
	t.gas -= amount

	return nil
}

func (t *Transition) buyGas() error {
	mgval := new(big.Int).Mul(new(big.Int).SetUint64(t.msg.Gas()), t.msg.GasPrice())
	if t.state.GetBalance(t.msg.From()).Cmp(mgval) < 0 {
		return errInsufficientBalanceForGas
	}
	if err := t.gp.SubGas(t.msg.Gas()); err != nil {
		return err
	}
	t.gas += t.msg.Gas()

	t.initialGas = t.msg.Gas()
	t.state.SubBalance(t.msg.From(), mgval)
	return nil
}

func (t *Transition) preCheck() error {
	// Make sure this transaction's nonce is correct.
	if t.msg.CheckNonce() {
		nonce := t.state.GetNonce(t.msg.From())
		if nonce < t.msg.Nonce() {
			return fmt.Errorf("too high %d < %d", nonce, t.msg.Nonce())
		} else if nonce > t.msg.Nonce() {
			return fmt.Errorf("too low %d > %d", nonce, t.msg.Nonce())
		}
	}
	return t.buyGas()
}

func (t *Transition) Apply() error {
	if err := t.preCheck(); err != nil {
		return err
	}

	homestead := t.config.IsHomestead(t.env.Number)
	contractCreation := t.msg.To() == nil

	sender := t.msg.From()

	gas, err := IntrinsicGas(t.msg.Data(), contractCreation, homestead)
	if err != nil {
		return err
	}

	if err = t.useGas(gas); err != nil {
		return err
	}

	e := evm.NewEVM(t.state, t.env, t.config, t.config.GasTable(t.env.Number), vmTestBlockHash)

	var vmerr error
	if contractCreation {
		_, t.gas, vmerr = e.Create(sender, t.msg.Data(), t.msg.Value(), t.gas)
	} else {
		t.state.SetNonce(t.msg.From(), t.state.GetNonce(t.msg.From())+1)
		_, t.gas, vmerr = e.Call(sender, *t.msg.To(), t.msg.Data(), t.msg.Value(), t.gas)
	}

	if vmerr != nil {
		fmt.Printf("ERR: %s\n", vmerr)

		if vmerr == evm.ErrNotEnoughFunds {
			return vmerr
		}
	}

	fmt.Printf("Returned gas: %d\n", t.gas)
	fmt.Printf("Total gas consumed: %d\n", t.gasUsed())
	fmt.Printf("Refund: %d\n", t.state.GetRefund())

	t.refundGas()

	t.state.AddBalance(t.env.Coinbase, new(big.Int).Mul(new(big.Int).SetUint64(t.gasUsed()), t.msg.GasPrice()))

	return nil
}

func (t *Transition) refundGas() {
	// Apply refund counter, capped to half of the used gas.
	refund := t.gasUsed() / 2

	if refund > t.state.GetRefund() {
		refund = t.state.GetRefund()
	}

	t.gas += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(t.gas), t.msg.GasPrice())
	t.state.AddBalance(t.msg.From(), remaining)

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	t.gp.AddGas(t.gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (t *Transition) gasUsed() uint64 {
	return t.initialGas - t.gas
}

func RunSpecificTest(t *testing.T, c stateCase, id, fork string, index int, p postEntry) {
	fmt.Printf("%s: %s %d\n", id, fork, index)

	fmt.Println("-- entry --")
	fmt.Println(fork)
	fmt.Println(index)

	config, ok := Forks[fork]
	if !ok {
		t.Fatalf("config %s not found", fork)
	}

	env := c.Env.ToEnv(t)

	msg, err := c.Transaction.At(p.Indexes)
	if err != nil {
		t.Fatal(err)
	}
	env.GasPrice = msg.GasPrice()

	state := buildState(t, c.Pre)

	gaspool := new(core.GasPool)
	gaspool.AddGas(env.GasLimit.Uint64())

	tt := &Transition{
		state:  state,
		env:    env,
		config: config,
		msg:    msg,
		gp:     gaspool,
	}

	snapshot := state.Snapshot()
	if err := tt.Apply(); err != nil {
		state.RevertToSnapshot(snapshot)
	}

	fmt.Printf("Refund: %d\n", state.GetRefund())

	state.AddBalance(env.Coinbase, new(big.Int))

	root := state.IntermediateRoot(config.IsEIP158(env.Number))

	fmt.Printf("Found: %s\n", root.String())
	fmt.Printf("Expected: %s\n", p.Root.String())

	state.PrintObjects()

	if root != p.Root {
		t.Fatal("d")
	}

	if logs := rlpHash(state.Logs()); logs != common.Hash(p.Logs) {
		t.Fatal("also a mismatch")
	}
}

var skip2 = []string{
	"Call1024BalanceTooLow",
	"CallRecursiveBombPreCall",
	"Callcode1024BalanceTooLow",

	"stDelegatecallTestHomestead/Delegatecall1024",

	"CALLCODE_Bounds4",
	"CALL_Bounds2a",
	"CALL_Bounds3",
	"randomStatetest189",

	"stQuadraticComplexityTest",

	"randomStatetest246",
	"randomStatetest248",
	"randomStatetest375",
	"randomStatetest85",
	"randomStatetest579",
	"randomStatetest618",
	"randomStatetest626",
	"randomStatetest642",
	"randomStatetest644",
	"randomStatetest645",

	"modexp_modsize0_returndatasize",

	"call_ecrec_success_empty_then_returndatasize",
	"create_callprecompile_returndatasize",

	"LoopCallsDepthThenRevert2",

	"stRevertTest",

	"TestCryptographicFunctions",
	"failed_tx_xcf416c53",

	"static_Call1024PreCalls2",
	"static_Call50000_rip160",
	"static_Call50000_sha256",
	"static_CallEcrecover",
	"static_CallRipemd",
	"static_CallIdentity",
	"static_CallIdentitiy_1",
	"static_CallSha256",
	"stStaticCall",

	"stZeroKnowledge",
}

func skipTest2(name string) bool {
	for _, i := range skip2 {
		if strings.Contains(name, i) {
			return true
		}
	}
	return false
}

func RunTest(t *testing.T, id string, c stateCase) {

	for fork, f := range c.Post {
		for indx, e := range f {

			if fork != "Byzantium" { // Only do the byzantium for now
				continue
			}

			if indx != 0 {
				continue
			}

			fmt.Println("-- fork --")
			fmt.Println(fork)

			// run all the tests
			RunSpecificTest(t, c, id, fork, indx, e)
		}
	}
}

func TestTwo(t *testing.T) {

	files, err := listFiles("GeneralStateTests/stPreCompiledContracts2")
	if err != nil {
		panic(err)
	}

	fmt.Println(len(files))

	count := len(files)
	for indx, file := range files {

		if !strings.HasSuffix(file, ".json") {
			continue
		}
		id := fmt.Sprintf(" @@@@@@@@@@@@@@@ ==> %d|%d: %s", indx, count, file)

		data, err := ioutil.ReadFile(file)
		if err != nil {
			panic(err)
		}

		var c map[string]stateCase
		if err := json.Unmarshal(data, &c); err != nil {
			panic(err)
		}

		for _, i := range c {

			if skipTest2(id) {
				continue
			}

			RunTest(t, id, i)
		}
		// break

	}
}
