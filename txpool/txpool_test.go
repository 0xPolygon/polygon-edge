package txpool

/*
import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/state"
	itrie "github.com/0xPolygon/minimal/state/immutable-trie"
	"github.com/0xPolygon/minimal/types"
	"github.com/0xPolygon/minimal/types/buildroot"
)

var key1, _ = crypto.GenerateKey()
var addr1 = crypto.PubKeyToAddress(&key1.PublicKey)

var key2, _ = crypto.GenerateKey()
var addr2 = crypto.PubKeyToAddress(&key2.PublicKey)

func buildState(t *testing.T, allocs chain.GenesisAlloc) (state.State, state.Snapshot) {
	st := itrie.NewState(itrie.NewMemoryStorage())
	snap := st.NewSnapshot()

	txn := state.NewTxn(st, snap)

	for addr, alloc := range allocs {
		txn.CreateAccount(addr)
		txn.SetNonce(addr, alloc.Nonce)

		if alloc.Balance != nil {
			txn.SetBalance(addr, alloc.Balance)
		}
	}
	s, _ := txn.Commit(false)
	return st, s
}

var signer = &crypto.FrontierSigner{}

func TestTxPool(t *testing.T) {
	t.Skip()

	st, snap := buildState(t, chain.GenesisAlloc{
		addr1: chain.GenesisAccount{
			Nonce: 10,
		},
	})

	pool := NewTxPool(nil)

	txn := state.NewTxn(st, snap)
	pool.Update(nil, txn)

	err := pool.Add(buildTxn(11, key1))
	fmt.Println(err)
}

func buildTxn(nonce uint64, key *ecdsa.PrivateKey) *types.Transaction {

	addr := types.StringToAddress("0")
	txn := &types.Transaction{
		Nonce:    nonce,
		To:       &addr,
		Value:    []byte{0x1},
		Gas:      10,
		GasPrice: []byte{0x1},
		Input:    []byte{},
	}

	tx, err := signer.SignTx(txn, key)
	if err != nil {
		panic(err)
	}
	return tx
}

type dummyChain struct {
	headers map[string]*types.Block
}

func newDummyChain(headers []*header) (*dummyChain, error) {
	c := &dummyChain{
		headers: map[string]*types.Block{},
	}
	for _, h := range headers {
		if err := c.add(h); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *dummyChain) headerByHash(hash string) (*types.Block, bool) {
	h, ok := c.headers[hash]
	return h, ok
}

func (c *dummyChain) headersByNumber(number uint64) []*types.Block {
	res := []*types.Block{}
	for name, hh := range c.headers {
		if strings.Split(name, "-")[0] == fmt.Sprintf("%v", number) {
			res = append(res, hh)
		}
	}
	return res
}

func (c *dummyChain) add(h *header) error {
	hash := h.Hash()

	if _, ok := c.headers[hash]; ok {
		return fmt.Errorf("hash already imported")
	}

	var parent types.Hash
	if h.number != 0 {
		// Try to query parent at the specific fork
		header, ok := c.headers[fmt.Sprintf("%v-%v", h.number-1, h.fork)]
		if ok {
			parent = header.Hash()
		} else {
			// Get all the blocks with the parent number.
			// If there is only one block with the parent number we use that one
			parents := c.headersByNumber(h.number - 1)
			if len(parents) == 0 {
				return fmt.Errorf("parent not found")
			}
			if len(parents) != 1 {
				return fmt.Errorf("multiple parents %d. Specify the fork", len(parents))
			}
			parent = parents[0].Hash()
		}
	}

	header := &types.Header{
		ParentHash: parent,
		Number:     h.number,
		Difficulty: h.diff,
		TxRoot:     buildroot.CalculateTransactionsRoot(h.txs),
		ExtraData:  []byte{h.hash},
	}

	c.headers[hash] = generateNewBlock(header, h.txs, nil)
	return nil
}

type header struct {
	hash   byte
	fork   string
	parent byte
	number uint64
	diff   uint64
	txs    []*types.Transaction
}

func (h *header) Parent(parent byte) *header {
	h.parent = parent
	h.number = uint64(parent) + 1
	return h
}

func (h *header) Hash() string {
	return fmt.Sprintf("%v-%v", h.hash, h.fork)
}

func (h *header) Fork(fork string) *header {
	h.fork = fork
	return h
}

func (h *header) Txs(txs []*types.Transaction) *header {
	h.txs = txs
	return h
}

func mock(number byte) *header {
	return &header{
		hash:   number,
		fork:   "A",
		parent: number - 1,
		number: uint64(number),
		diff:   uint64(number),
	}
}

func TestTxPoolReset(t *testing.T) {
	t.Skip()

	cases := []struct {
		PreState  chain.GenesisAlloc // state after the reorg
		History   []*header
		OldHeader string
		NewHeader string
		Expected  []*types.Transaction
	}{
		{
			PreState: chain.GenesisAlloc{
				addr1: chain.GenesisAccount{
					Nonce: 2,
				},
			},
			History: []*header{
				mock(0x0),

				// Fork A
				mock(0x1).Txs([]*types.Transaction{
					buildTxn(0, key1),
					buildTxn(1, key1),
				}),
				mock(0x2).Txs([]*types.Transaction{
					buildTxn(2, key1),
					buildTxn(3, key1),
				}),

				// Fork B
				mock(0x1).Fork("B"),
				mock(0x2).Fork("B").Txs([]*types.Transaction{
					buildTxn(0, key1),
				}),
			},
			OldHeader: mock(0x2).Hash(),
			NewHeader: mock(0x2).Fork("B").Hash(),
			Expected: []*types.Transaction{
				buildTxn(2, key1),
				buildTxn(3, key1),
			},
		},
	}

	for _, c := range cases {
		st, snap := buildState(t, c.PreState)
		txn := state.NewTxn(st, snap)

		chain, err := newDummyChain(c.History)
		if err != nil {
			panic(err)
		}

		b := blockchain.NewTestBlockchain(t, nil)

		// genesis is 0x0
		if err := b.WriteHeaderGenesis(chain.headers[c.History[0].Hash()].Header); err != nil {
			t.Fatal(err)
		}

		// run the history
		for i := 1; i < len(c.History); i++ {
			block := chain.headers[c.History[i].Hash()]
			if err := b.WriteHeader(block.Header); err != nil {
				t.Fatal(err)
			}
			// b.WriteAuxBlocks(block)
		}

		pool := NewTxPool(b)

		old, ok := chain.headerByHash(mock(0x2).Hash())
		if !ok {
			t.Fatalf("old header not found")
		}
		new, ok := chain.headerByHash(mock(0x2).Fork("B").Hash())
		if !ok {
			t.Fatalf("new header not found")
		}

		pool.state = txn
		promoted, err := pool.reset(old.Header, new.Header)
		if err != nil {
			t.Fatal(err)
		}

		if len(promoted) != len(c.Expected) {
			t.Fatalf("length is not the same: expected %d but found %d", len(c.Expected), len(promoted))
		}
		if len(txDifference(promoted, c.Expected)) != 0 {
			t.Fatalf("bad")
		}
	}
}

func TestTxPoolValidateTx(t *testing.T) {
	// TODO: Validate tx
}

func TestTxQueuePromotion(t *testing.T) {
	t.Skip()

	cases := []struct {
		Nonces   []uint64
		Promote  uint64
		Promoted []uint64
	}{
		{
			Nonces: []uint64{
				10, 5, 4, 1,
			},
			Promote: 4,
			Promoted: []uint64{
				4, 5,
			},
		},
		{
			Nonces: []uint64{
				1, 2, 3, 4,
			},
			Promote:  5,
			Promoted: []uint64{},
		},
		{
			Nonces: []uint64{
				3, 5, 6, 7, 4,
			},
			Promote: 3,
			Promoted: []uint64{
				3, 4, 5, 6, 7,
			},
		},
		{
			Nonces: []uint64{
				3, 4, 5, 6,
			},
			Promote:  2,
			Promoted: []uint64{},
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			q := newTxQueue()
			for _, nonce := range c.Nonces {
				q.Add(buildTxn(nonce, key1))
			}

			promoted := []uint64{}
			for _, tx := range q.Promote(c.Promote) {
				promoted = append(promoted, tx.Nonce)
			}

			if !reflect.DeepEqual(promoted, c.Promoted) {
				t.Fatal("bad")
			}
		})
	}
}

func TestPricedTxs(t *testing.T) {
	t.Skip()

	pool := newTxPriceHeap()

	if err := pool.Push(addr1, buildTxn(1, key1), big.NewInt(100)); err != nil {
		t.Fatal(err)
	}
	if err := pool.Push(addr1, buildTxn(2, key1), big.NewInt(1000)); err != nil {
		t.Fatal(err)
	}
	if err := pool.Push(addr2, buildTxn(3, key2), big.NewInt(1001)); err != nil {
		t.Fatal(err)
	}

	if nonce := pool.Pop().tx.Nonce; nonce != 3 {
		t.Fatalf("expected nonce 3 but found: %d", nonce)
	}
	if nonce := pool.Pop().tx.Nonce; nonce != 1 {
		t.Fatalf("expected nonce 1 but found: %d", nonce)
	}
	if nonce := pool.Pop().tx.Nonce; nonce != 2 {
		t.Fatalf("expected nonce 2 but found: %d", nonce)
	}
	if pool.Pop() != nil {
		t.Fatal("not expected any other element")
	}
}
*/
