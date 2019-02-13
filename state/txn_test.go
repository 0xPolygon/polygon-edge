package state

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/evm"
)

var addr1 = common.HexToAddress("1")
var addr2 = common.HexToAddress("2")

var hash0 = common.HexToHash("0")
var hash1 = common.HexToHash("1")
var hash2 = common.HexToHash("2")

var defaultPreState = map[common.Address]*PreState{
	addr1: {
		State: map[common.Hash]common.Hash{
			hash1: hash1,
		},
	},
}

func buildPreState(s *State, preState map[common.Address]*PreState) {
	txn := s.Txn()
	for i, j := range preState {
		txn.SetNonce(i, j.Nonce)
		txn.SetBalance(i, big.NewInt(int64(j.Balance)))
		for k, v := range j.State {
			txn.SetState(i, k, v)
		}
	}
	txn.Commit(false)
}

type PreState struct {
	Nonce   uint64
	Balance uint64
	State   map[common.Hash]common.Hash
}

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
			s := NewState()
			buildPreState(s, c.PreState)

			txn := s.Txn()
			_, _, err := txn.Apply(c.Transaction.ToMessage(), &evm.Env{}, chain.GasTableHomestead, chain.ForksInTime{}, vmTestBlockHash, newGasPool(1000), true)

			if err != nil {
				if c.Err == "" {
					t.Fatalf("Error not expected: %v", err)
				}
				if c.Err != err.Error() {
					t.Fatalf("Errors dont match: %s and %v", c.Err, err)
				}
			} else if c.Err != "" {
				t.Fatalf("It did not failed (%s)", c.Err)
			}
		})
	}
}

func TestWriteState(t *testing.T) {
	// write new state
	s := NewState()
	txn := s.Txn()

	txn.SetState(addr1, hash1, hash1)
	txn.SetState(addr1, hash2, hash2)

	txn.Commit(false)

	txn = s.Txn()
	if txn.GetState(addr1, hash1) != hash1 {
		t.Fail()
	}
	if txn.GetState(addr1, hash2) != hash2 {
		t.Fail()
	}
}

func TestWriteEmptyState(t *testing.T) {
	// Create account and write empty state
	s := NewState()
	txn := s.Txn()

	// Without EIP150 the data is added
	txn.SetState(addr1, hash1, hash0)
	txn.Commit(false)

	txn = s.Txn()
	if !txn.Exist(addr1) {
		t.Fatal()
	}

	s = NewState()
	txn = s.Txn()

	// With EIP150 the empty data is removed
	txn.SetState(addr1, hash1, hash0)
	txn.Commit(true)

	txn = s.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestUpdateStateInPreState(t *testing.T) {
	// update state that was already set in prestate
	s := NewState()
	buildPreState(s, defaultPreState)

	txn := s.Txn()
	if txn.GetState(addr1, hash1) != hash1 {
		t.Fatal()
	}

	txn.SetState(addr1, hash1, hash2)
	txn.Commit(false)

	txn = s.Txn()
	if txn.GetState(addr1, hash1) != hash2 {
		t.Fatal()
	}
}

func TestUpdateStateWithEmpty(t *testing.T) {
	// If the state (in prestate) is updated to empty it should be removed
	s := NewState()
	buildPreState(s, defaultPreState)

	txn := s.Txn()
	txn.SetState(addr1, hash1, hash0)

	// TODO, test with false (should not be deleted)
	// TODO, test with balance on the account and nonce
	txn.Commit(true)

	txn = s.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestSuicideAccountInPreState(t *testing.T) {
	// Suicide an account created in the prestate
	s := NewState()
	buildPreState(s, defaultPreState)

	txn := s.Txn()
	txn.Suicide(addr1)
	txn.Commit(true)

	txn = s.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestSuicideAccount(t *testing.T) {
	// Create a new account and suicide it
	s := NewState()

	txn := s.Txn()
	txn.SetState(addr1, hash1, hash1)
	txn.Suicide(addr1)

	// Note, even if has commit suicide it still exists in the current txn
	if !txn.Exist(addr1) {
		t.Fatal()
	}

	txn.Commit(true)

	txn = s.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestSuicideAccountWithData(t *testing.T) {
	// Data (nonce, balance, code) from a suicided account should be empty
	s := NewState()
	txn := s.Txn()

	txn.SetNonce(addr1, 10)
	txn.SetBalance(addr1, big.NewInt(100))
	txn.SetCode(addr1, []byte{0x1, 0x2, 0x3})
	txn.SetState(addr1, hash1, hash1)

	txn.Suicide(addr1)
	txn.Commit(true)

	txn = s.Txn()

	if balance := txn.GetBalance(addr1); balance.Cmp(big.NewInt(0)) != 0 {
		t.Fatalf("balance should be zero but found: %d", balance)
	}
	if nonce := txn.GetNonce(addr1); nonce != 0 {
		t.Fatalf("nonce should be zero but found %d", nonce)
	}
	if code := txn.GetCode(addr1); len(code) != 0 {
		t.Fatalf("code should be empty but found: %s", hexutil.Encode(code))
	}
	if codeHash := txn.GetCodeHash(addr1); codeHash != (common.Hash{}) {
		t.Fatalf("code hash should be empty but found: %s", codeHash.String())
	}
	if size := txn.GetCodeSize(addr1); size != 0 {
		t.Fatalf("code size should be zero but found %d", size)
	}
	if value := txn.GetState(addr1, hash1); value != (common.Hash{}) {
		t.Fatalf("value should be empty but found: %s", value.String())
	}
}

func TestSuicideCoinbase(t *testing.T) {
	// Suicide the coinbase of the block
	s := NewState()
	buildPreState(s, defaultPreState)

	txn := s.Txn()
	txn.Suicide(addr1)
	txn.AddSealingReward(addr1, big.NewInt(10))
	txn.Commit(true)

	txn = s.Txn()
	if txn.GetBalance(addr1).Cmp(big.NewInt(10)) != 0 {
		t.Fatal()
	}
}

func TestSuicideWithIntermediateCommit(t *testing.T) {
	s := NewState()

	txn := s.Txn()
	txn.SetNonce(addr1, 10)
	txn.Suicide(addr1)

	if txn.GetNonce(addr1) != 10 { // it is still 'active'
		t.Fatal()
	}

	txn.IntermediateCommit(true)

	if txn.GetNonce(addr1) == 10 {
		t.Fatal()
	}

	txn.Commit(true)
	if txn.GetNonce(addr1) == 10 {
		t.Fatal()
	}
}

func TestRestartRefunds(t *testing.T) {
	// refunds are only valid per single txn so after each
	// intermediateCommit they have to be restarted
	s := NewState()
	txn := s.Txn()

	txn.AddRefund(1000)
	if txn.GetRefund() != 1000 {
		t.Fatal()
	}

	txn.IntermediateCommit(false)
	if refunds := txn.GetRefund(); refunds == 1000 {
		t.Fatalf("refunds should be empty buf founds: %d", refunds)
	}
}

func TestChangePrestateAccountBalanceToZero(t *testing.T) {
	// If the balance of the account changes to zero the account is deleted
	preState := map[common.Address]*PreState{
		addr1: {
			Balance: 10,
		},
	}

	s := NewState()
	buildPreState(s, preState)

	txn := s.Txn()
	txn.SetBalance(addr1, big.NewInt(0))
	txn.Commit(true)

	txn = s.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestChangeAccountBalanceToZero(t *testing.T) {
	// If the balance of the account changes to zero the account is deleted
	s := NewState()

	txn := s.Txn()
	txn.SetBalance(addr1, big.NewInt(10))
	txn.SetBalance(addr1, big.NewInt(0))
	txn.Commit(true)

	txn = s.Txn()
	if txn.Exist(addr1) {
		t.Fatal()
	}
}

func TestSnapshotUpdateData(t *testing.T) {
	// Snapshots should keep the data

	s := NewState()
	txn := s.Txn()

	txn.SetState(addr1, hash1, hash1)
	if txn.GetState(addr1, hash1) != hash1 {
		t.Fail()
	}

	ss := txn.Snapshot()
	txn.SetState(addr1, hash1, hash2)
	if txn.GetState(addr1, hash1) != hash2 {
		t.Fail()
	}

	txn.RevertToSnapshot(ss)
	if txn.GetState(addr1, hash1) != hash1 {
		t.Fail()
	}
}
