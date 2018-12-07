package tests

import (
	"encoding/json"
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
		panic("1")
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
			panic("toto ghi")
		} else if nonce > t.msg.Nonce() {
			panic("too low")
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

	if contractCreation {
		_, t.gas, err = e.Create(sender, t.msg.Data(), t.msg.Value(), t.gas)
	} else {
		t.state.SetNonce(t.msg.From(), t.state.GetNonce(sender)+1)
		_, t.gas, err = e.Call(sender, *t.msg.To(), t.msg.Data(), t.msg.Value(), t.gas)
	}
	if err != nil {
		panic(err)
	}

	fmt.Printf("Three: %s\n", t.state.IntermediateRoot(true).String())

	fmt.Printf("Returned gas: %d\n", t.gas)
	fmt.Printf("Total gas consumed: %d\n", t.gasUsed())

	t.refundGas()
	t.state.AddBalance(t.env.Coinbase, new(big.Int).Mul(new(big.Int).SetUint64(t.gasUsed()), t.msg.GasPrice()))

	fmt.Printf("Four: %s\n", t.state.IntermediateRoot(true).String())

	t.state.PrintObjects()

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

	tt.Apply()

	fmt.Printf("Root after: %s\n", state.IntermediateRoot(config.IsEIP158(env.Number)).String())

	// commit the data
	// and also add the reward, now only assumes the reward goes to the coinbase one

	fmt.Println("EIP-158")
	fmt.Println(config.IsEIP158(env.Number))

	/*
		if _, err := state.Commit(config.IsEIP158(env.Number)); err != nil {
			panic(err)
		}
	*/

	state.AddBalance(env.Coinbase, new(big.Int))

	root := state.IntermediateRoot(config.IsEIP158(env.Number))

	fmt.Printf("Root: %s\n", root.String())
	fmt.Printf("Other root: %s\n", p.Root.String())

	if root != p.Root {
		t.Fatal("d")
	}

	if logs := rlpHash(state.Logs()); logs != common.Hash(p.Logs) {
		t.Fatal("also a mismatch")
	}
}

var skip2 = []string{
	"suicideNonConst",
	"ContractCreationSpam",
	"CrashingTransaction",
	"returndatacopyPythonBug_Tue_03_48_41-1432",
	"stBugs",
	"stCallCodes",
	"stCallCreateCallCodeTest",
	"stCallDelegateCodesCallCodeHomestead",
}

// not sure yet how to catch the bad cases

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

			if fork != "Byzantium" {
				continue
			}

			if indx != 0 {
				continue
			}

			fmt.Println("-- fork --")
			fmt.Println(fork)

			// run all the tests
			RunSpecificTest(t, c, id, fork, indx, e)

			// return
		}
	}
}

func TestTwo(t *testing.T) {

	files, err := listFiles("GeneralStateTests")
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
