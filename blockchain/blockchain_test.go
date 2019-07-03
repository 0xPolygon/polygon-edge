package blockchain

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umbracle/minimal/chain"

	"github.com/umbracle/minimal/types"
)

func TestGenesis(t *testing.T) {
	b := NewTestBlockchain(t, nil)

	// no genesis block yet
	if _, ok := b.Header(); ok {
		t.Fatal("it should be empty")
	}

	// add genesis block
	genesis := &types.Header{Difficulty: 1, Number: 0}
	assert.NoError(t, b.WriteHeaderGenesis(genesis))

	header, _ := b.Header()
	assert.Equal(t, header.Hash(), genesis.Hash())
}

func TestChainGenesis(t *testing.T) {
	// Test chain genesis from json files
	cases := []struct {
		Name string
		Root string
		Hash string
	}{
		{
			Name: "foundation",
			Root: "0xd7f8974fb5ac78d9ac099b9ad5018bedc2ce0a72dad1827a1709da30580f0544",
			Hash: "0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			genesis, err := chain.ImportFromName(c.Name)
			assert.NoError(t, err)

			b := NewTestBlockchain(t, nil)
			assert.NoError(t, b.WriteGenesis(genesis.Genesis))

			root := b.genesis.StateRoot.String()
			assert.Equal(t, root, c.Root)
			assert.Equal(t, b.genesis.Hash().String(), c.Hash)
		})
	}
}

type dummyChain struct {
	headers map[byte]*types.Header
}

func (c *dummyChain) add(h *header) error {
	if _, ok := c.headers[h.hash]; ok {
		return fmt.Errorf("hash already imported")
	}

	var parent types.Hash
	if h.number != 0 {
		p, ok := c.headers[h.parent]
		if !ok {
			return fmt.Errorf("parent not found %v", h.parent)
		}
		parent = p.Hash()
	}

	c.headers[h.hash] = &types.Header{
		ParentHash: parent,
		Number:     h.number,
		Difficulty: h.diff,
		ExtraData:  []byte{h.hash},
	}
	return nil
}

type header struct {
	hash   byte
	parent byte
	number uint64
	diff   uint64
}

func (h *header) Parent(parent byte) *header {
	h.parent = parent
	h.number = uint64(parent) + 1
	return h
}

func (h *header) Diff(d uint64) *header {
	h.diff = d
	return h
}

func (h *header) Number(d uint64) *header {
	h.number = d
	return h
}

func mock(number byte) *header {
	return &header{
		hash:   number,
		parent: number - 1,
		number: uint64(number),
		diff:   uint64(number),
	}
}

func TestInsertHeaders(t *testing.T) {
	var cases = []struct {
		Name    string
		History []*header
		Head    *header
		Forks   []*header
		Chain   []*header
		TD      uint64
	}{
		{
			Name: "Genesis",
			History: []*header{
				mock(0x0),
			},
			Head: mock(0x0),
			Chain: []*header{
				mock(0x0),
			},
			TD: 1,
		},
		{
			Name: "Linear",
			History: []*header{
				mock(0x0),
				mock(0x1),
				mock(0x2),
			},
			Head: mock(0x2),
			Chain: []*header{
				mock(0x0),
				mock(0x1),
				mock(0x2),
			},
			TD: 1 + 1 + 2,
		},
		{
			Name: "Keep block with higher difficulty",
			History: []*header{
				mock(0x0),
				mock(0x1),
				mock(0x3).Parent(0x1).Diff(5),
				mock(0x2).Parent(0x1).Diff(3),
			},
			Head:  mock(0x3),
			Forks: []*header{mock(0x2)},
			Chain: []*header{
				mock(0x0),
				mock(0x1),
				mock(0x3).Parent(0x1).Diff(5),
			},
			TD: 1 + 1 + 5,
		},
		{
			Name: "Reorg",
			History: []*header{
				mock(0x0),
				mock(0x1),
				mock(0x2),
				mock(0x3),
				mock(0x4).Parent(0x1).Diff(10).Number(2),
				mock(0x5).Parent(0x4).Diff(11).Number(3),
				mock(0x6).Parent(0x3).Number(4),
			},
			Head:  mock(0x5),
			Forks: []*header{mock(0x6)},
			Chain: []*header{
				mock(0x0),
				mock(0x1),
				mock(0x4).Parent(0x1).Diff(10).Number(2),
				mock(0x5).Parent(0x4).Diff(11).Number(3),
			},
			TD: 1 + 1 + 10 + 11,
		},
		{
			Name: "Forks in reorgs",
			History: []*header{
				mock(0x0),
				mock(0x1),
				mock(0x2),
				mock(0x3), // fork because of the 0x4 reorg
				mock(0x4).Parent(0x2).Diff(11),
				mock(0x5).Parent(0x3),         // replace 0x3 as header fork
				mock(0x6).Parent(0x2).Diff(5), // lower fork in 0x1
			},
			Head:  mock(0x4),
			Forks: []*header{mock(0x5), mock(0x6)},
			Chain: []*header{
				mock(0x0),
				mock(0x1),
				mock(0x2),
				mock(0x4).Parent(0x2).Diff(11),
			},
			TD: 1 + 1 + 2 + 11,
		},
	}

	for _, cc := range cases {
		t.Run(cc.Name, func(tt *testing.T) {
			b := NewTestBlockchain(t, nil)

			chain := dummyChain{
				headers: map[byte]*types.Header{},
			}
			for _, i := range cc.History {
				if err := chain.add(i); err != nil {
					tt.Fatal(err)
				}
			}

			// genesis is 0x0
			if err := b.WriteHeaderGenesis(chain.headers[0x0]); err != nil {
				tt.Fatal(err)
			}

			// run the history
			for i := 1; i < len(cc.History); i++ {
				if err := b.WriteHeader(chain.headers[cc.History[i].hash]); err != nil {
					tt.Fatal(err)
				}
			}

			head, _ := b.Header()

			expected, ok := chain.headers[cc.Head.hash]
			assert.True(t, ok)

			// check that we got the right hash
			assert.Equal(t, head.Hash(), expected.Hash())

			forks := b.GetForks()
			expectedForks := []types.Hash{}

			for _, i := range cc.Forks {
				expectedForks = append(expectedForks, chain.headers[i.hash].Hash())
			}

			if len(forks) != 0 {
				if len(forks) != len(expectedForks) {
					tt.Fatalf("forks length dont match, expected %d but found %d", len(expectedForks), len(forks))
				} else {
					if !reflect.DeepEqual(forks, expectedForks) {
						tt.Fatal("forks dont match")
					}
				}
			}

			// Check chain of forks
			if cc.Chain != nil {
				for indx, i := range cc.Chain {
					block, _ := b.GetBlockByNumber(uint64(indx), true)
					if block.Hash().String() != chain.headers[i.hash].Hash().String() {
						tt.Fatal("bad")
					}
				}
			}

			if td, _ := b.GetChainTD(); cc.TD != td.Uint64() {
				tt.Fatal("bad")
			}
		})
	}
}

func TestForkUnkwonParents(t *testing.T) {
	b := NewTestBlockchain(t, nil)

	h0 := NewTestHeaderChain(10)
	h1 := NewTestHeaderFromChain(h0[:5], 10)

	// Write genesis
	assert.NoError(t, b.WriteHeaderGenesis(h0[0]))

	// Write 10 headers
	assert.NoError(t, b.WriteHeaders(h0[1:]))

	// Cannot write this header because the father h1[11] is not known
	assert.Error(t, b.WriteHeader(h1[12]))

}

func TestCommitChain(t *testing.T) {
	// test if the data written in commitchain is retrieved correctly

	headers, blocks, receipts := NewTestBodyChain(2)
	b := NewTestBlockchain(t, headers)

	// commit values to the chain, skip first block (genesis)
	assert.NoError(t, b.CommitChain(blocks, receipts))

	for i := 1; i < len(blocks); i++ {
		block := blocks[i]

		// check blocks
		i, ok := b.db.ReadBody(block.Hash())
		if !ok {
			t.Fatal("it should exists")
		}

		assert.Len(t, i.Transactions, 1)
		assert.Equal(t, i.Transactions[0].Nonce, block.Number())

		// check receipts
		r := b.db.ReadReceipts(block.Hash())
		assert.Len(t, r, 1)
	}
}
