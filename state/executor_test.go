package state

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/runtime"
	"github.com/umbracle/minimal/state/runtime/evm"
)

func TestExecutor(t *testing.T) {
	fmt.Println("-- executor --")

	addr1 := common.HexToAddress("1")
	addr2 := common.HexToAddress("2")

	s := NewState()
	txn := s.Txn()

	env := &runtime.Env{}

	e := NewExecutor(txn, env, chain.ForksInTime{}, chain.GasTableHomestead, nil)

	fmt.Println(e)

	input := []byte{
		byte(evm.CALL),
	}
	c := runtime.NewContract(addr1, addr1, addr2, big.NewInt(100), 100, input)

	ret, gas, err := e.Call(c)

	fmt.Println(ret)
	fmt.Println(gas)
	fmt.Println(err)

}
