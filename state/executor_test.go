package state

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/runtime"
)

type Transaction struct {
	From     common.Address
	To       common.Address
	Nonce    uint64
	Amount   uint64
	GasLimit uint64
	GasPrice uint64
	Data     []byte
}

func (t *Transaction) ToMessage() *types.Message {
	msg := types.NewMessage(t.From, &t.To, t.Nonce, big.NewInt(int64(t.Amount)), t.GasLimit, big.NewInt(int64(t.GasPrice)), t.Data, true)
	return &msg
}

func vmTestBlockHash(n uint64) common.Hash {
	return common.BytesToHash(crypto.Keccak256([]byte(big.NewInt(int64(n)).String())))
}

type gasPool struct {
	gas uint64
}

func (g *gasPool) SubGas(amount uint64) error {
	if g.gas < amount {
		return fmt.Errorf("gas limit reached")
	}
	g.gas -= amount
	return nil
}

func (g *gasPool) AddGas(amount uint64) {
	g.gas += amount
}

func newGasPool(gas uint64) *gasPool {
	return &gasPool{gas}
}

func TestTransition(t *testing.T) {
	addr1 := common.HexToAddress("1")

	type Case struct {
		PreState    map[common.Address]*PreState
		Transaction *Transaction
		Err         string
	}

	var cases = map[string]*Case{
		"Nonce too low": {
			PreState: map[common.Address]*PreState{
				addr1: {
					Nonce: 10,
				},
			},
			Transaction: &Transaction{
				From:  addr1,
				Nonce: 5,
			},
			Err: "too low 10 > 5",
		},
		"Nonce too high": {
			PreState: map[common.Address]*PreState{
				addr1: {
					Nonce: 5,
				},
			},
			Transaction: &Transaction{
				From:  addr1,
				Nonce: 10,
			},
			Err: "too high 5 < 10",
		},
		"Insuficient balance to pay gas": {
			PreState: map[common.Address]*PreState{
				addr1: {
					Balance: 50,
				},
			},
			Transaction: &Transaction{
				From:     addr1,
				GasLimit: 1,
				GasPrice: 100,
			},
			Err: ErrInsufficientBalanceForGas.Error(),
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			txn := newTestTxn(c.PreState)

			env := &runtime.Env{}
			config := chain.ForksInTime{}

			exec := NewExecutor(txn, env, config, chain.GasTableHomestead, vmTestBlockHash)

			_, _, err := exec.Apply(txn, c.Transaction.ToMessage(), env, chain.GasTableHomestead, config, vmTestBlockHash, newGasPool(1000), false, nil)
			if err != nil {
				if c.Err == "" {
					t.Fatalf("Error not expected: %v", err)
				}
				if c.Err != err.Error() {
					t.Fatalf("Errors dont match: expected '%s' but found '%v'", c.Err, err)
				}
			} else if c.Err != "" {
				t.Fatalf("It did not failed (%s)", c.Err)
			}
		})
	}
}
