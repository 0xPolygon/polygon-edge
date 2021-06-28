package tests

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"strings"
	"testing"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/state"
	itrie "github.com/0xPolygon/minimal/state/immutable-trie"
	"github.com/0xPolygon/minimal/state/runtime/evm"
	"github.com/0xPolygon/minimal/state/runtime/precompiled"
	"github.com/0xPolygon/minimal/types"
	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"
)

var (
	stateTests       = "GeneralStateTests"
	legacyStateTests = "LegacyTests/Constantinople/GeneralStateTests"
)

type stateCase struct {
	Info        *info                                   `json:"_info"`
	Env         *env                                    `json:"env"`
	Pre         map[types.Address]*chain.GenesisAccount `json:"pre"`
	Post        map[string]postState                    `json:"post"`
	Transaction *stTransaction                          `json:"transaction"`
}

var ripemd = types.StringToAddress("0000000000000000000000000000000000000003")

type accountTrie interface {
	Get(k []byte) ([]byte, bool)
}

type legacyAccount struct {
	Nonce    uint64
	Balance  *big.Int
	Root     types.Hash
	CodeHash []byte
	Trie     accountTrie
}

func (a *legacyAccount) marshalWith(ar *fastrlp.Arena) *fastrlp.Value {
	v := ar.NewArray()
	v.Set(ar.NewUint(a.Nonce))
	v.Set(ar.NewBigInt(a.Balance))
	v.Set(ar.NewBytes(a.Root.Bytes()))
	v.Set(ar.NewBytes(a.CodeHash))

	return v
}

var stateTestArenaPool fastrlp.ArenaPool

func hashIt(k []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(k)
	return h.Sum(nil)
}

type coinbaseParams struct {
	coinbaseAddress types.Address
	shouldInclude   bool
}

func pruneStakedAccounts(
	snapshot state.Snapshot,
	accountMap map[types.Address]*chain.GenesisAccount,
	cParms coinbaseParams,
) []byte {
	localTrie := itrie.NewTrie()
	trieTransaction := localTrie.Txn()

	arena := stateTestArenaPool.Get()
	defer stateTestArenaPool.Put(arena)

	var touchedAccounts []types.Address
	for accountAddress := range accountMap {
		touchedAccounts = append(touchedAccounts, accountAddress)
	}

	if cParms.shouldInclude {
		touchedAccounts = append(touchedAccounts, cParms.coinbaseAddress)

	}

	for _, accountAddress := range touchedAccounts {
		key := keccak.Keccak256(nil, accountAddress.Bytes())

		// Grab the account from storage, as it was committed earlier
		result, ok := snapshot.Get(key)
		if !ok {
			return nil
		}

		var account state.Account
		if err := account.UnmarshalRlp(result); err != nil {
			return nil
		}

		// Create a new legacy account object,
		// without the staked balance field
		legacyAcc := &legacyAccount{
			Nonce:    account.Nonce,
			Balance:  account.Balance,
			Root:     account.Root, // Storage root
			CodeHash: account.CodeHash,
			Trie:     account.Trie,
		}

		vv := legacyAcc.marshalWith(arena)
		data := vv.MarshalTo(nil)

		// Add the legacy account to the local trie
		trieTransaction.Insert(hashIt(accountAddress.Bytes()), data)
		arena.Reset()
	}

	newRoot, _ := trieTransaction.Hash()

	return newRoot
}

func RunSpecificTest(file string, t *testing.T, c stateCase, name, fork string, index int, p postEntry) {
	config, ok := Forks[fork]
	if !ok {
		t.Fatalf("config %s not found", fork)
	}

	env := c.Env.ToEnv(t)

	msg, err := c.Transaction.At(p.Indexes)
	if err != nil {
		t.Fatal(err)
	}

	s, _, pastRoot := buildState(t, c.Pre)
	forks := config.At(uint64(env.Number))

	xxx := state.NewExecutor(&chain.Params{Forks: config, ChainID: 1}, s)
	xxx.SetRuntime(precompiled.NewPrecompiled())
	xxx.SetRuntime(evm.NewEVM())

	xxx.PostHook = func(t *state.Transition) {
		if name == "failed_tx_xcf416c53" {
			// create the account
			t.Txn().TouchAccount(ripemd)
			// now remove it
			t.Txn().Suicide(ripemd)
		}
	}
	xxx.GetHash = func(*types.Header) func(i uint64) types.Hash {
		return vmTestBlockHash
	}

	executor, _ := xxx.BeginTxn(pastRoot, c.Env.ToHeader(t))
	executor.Apply(msg) //nolint:errcheck

	txn := executor.Txn()

	// mining rewards
	txn.AddSealingReward(env.Coinbase, big.NewInt(0))

	_, root := txn.Commit(forks.EIP158)
	if !bytes.Equal(root, p.Root.Bytes()) {
		t.Fatalf("root mismatch (%s %s %s %d): expected %s but found %s", file, name, fork, index, p.Root.String(), hex.EncodeToHex(root))
	}

	if logs := rlpHashLogs(txn.Logs()); logs != p.Logs {
		t.Fatalf("logs mismatch (%s, %s %d): expected %s but found %s", name, fork, index, p.Logs.String(), logs.String())
	}
}

func TestState(t *testing.T) {
	// The TestState is skipped for now, because the current
	// IBFT PoS implementation modifies the Account state object,
	// which in turn causes the Ethereum tests to fail (because of root hash mismatch)
	t.Skip()

	long := []string{
		"static_Call50000",
		"static_Return50000",
		"static_Call1MB",
		"stQuadraticComplexityTest",
		"stTimeConsuming",
	}

	skip := []string{
		"RevertPrecompiledTouch",
	}

	// There are two folders in spec tests, one for the current tests for the Istanbul fork
	// and one for the legacy tests for the other forks
	folders, err := listFolders(stateTests, legacyStateTests)
	if err != nil {
		t.Fatal(err)
	}

	for _, folder := range folders {
		t.Run(folder, func(t *testing.T) {
			files, err := listFiles(folder)
			if err != nil {
				t.Fatal(err)
			}

			for _, file := range files {
				if !strings.HasSuffix(file, ".json") {
					continue
				}

				if contains(long, file) && testing.Short() {
					t.Skipf("Long tests are skipped in short mode")
					continue
				}

				if contains(skip, file) {
					t.Skip()
					continue
				}

				data, err := ioutil.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}

				var c map[string]stateCase
				if err := json.Unmarshal(data, &c); err != nil {
					t.Fatal(err)
				}

				for name, i := range c {
					for fork, f := range i.Post {
						for indx, e := range f {
							RunSpecificTest(file, t, i, name, fork, indx, e)
						}
					}
				}
			}
		})
	}
}
