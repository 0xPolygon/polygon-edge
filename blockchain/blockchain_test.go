package blockchain

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/0xPolygon/minimal/chain"
	"github.com/stretchr/testify/assert"

	"github.com/0xPolygon/minimal/types"
)

func TestGenesis(t *testing.T) {
	b := NewTestBlockchain(t, nil)

	// add genesis block
	genesis := &types.Header{Difficulty: 1, Number: 0}
	assert.NoError(t, b.WriteHeaderGenesis(genesis))

	header := b.Header()
	assert.Equal(t, header.Hash, genesis.Hash)
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
			genesisConfig, err := chain.ImportFromName(c.Name)
			assert.NoError(t, err)

			b := NewTestBlockchain(t, nil)
			assert.NoError(t, b.WriteGenesis(genesisConfig.Genesis))

			genesisHeader, ok := b.GetHeaderByNumber(0)
			assert.True(t, ok)

			root := genesisHeader.StateRoot.String()
			assert.Equal(t, root, c.Root)
			assert.Equal(t, genesisHeader.Hash.String(), c.Hash)
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
		parent = p.Hash
	}

	hh := &types.Header{
		ParentHash: parent,
		Number:     h.number,
		Difficulty: h.diff,
		ExtraData:  []byte{h.hash},
	}
	hh.ComputeHash()
	c.headers[h.hash] = hh
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

	type evnt struct {
		NewChain []*header
		OldChain []*header
		Diff     *big.Int
	}
	type headerEvnt struct {
		header *header
		event  *evnt
	}

	var cases = []struct {
		Name    string
		History []*headerEvnt
		Head    *header
		Forks   []*header
		Chain   []*header
		TD      uint64
	}{
		{
			Name: "Genesis",
			History: []*headerEvnt{
				{
					header: mock(0x0),
				},
			},
			Head: mock(0x0),
			Chain: []*header{
				mock(0x0),
			},
			TD: 1,
		},
		{
			Name: "Linear",
			History: []*headerEvnt{
				{
					header: mock(0x0),
				},
				{
					header: mock(0x1),
					event: &evnt{
						NewChain: []*header{
							mock(0x1),
						},
						Diff: big.NewInt(2),
					},
				},
				{
					header: mock(0x2),
					event: &evnt{
						NewChain: []*header{
							mock(0x2),
						},
						Diff: big.NewInt(4),
					},
				},
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
			History: []*headerEvnt{
				{
					header: mock(0x0),
				},
				{
					header: mock(0x1),
					event: &evnt{
						NewChain: []*header{
							mock(0x1),
						},
						Diff: big.NewInt(2),
					},
				},
				{
					header: mock(0x3).Parent(0x1).Diff(5),
					event: &evnt{
						NewChain: []*header{
							mock(0x3).Parent(0x1).Diff(5),
						},
						Diff: big.NewInt(7),
					},
				},
				{
					// This block has lower difficulty than the current chain (fork)
					header: mock(0x2).Parent(0x1).Diff(3),
					event: &evnt{
						OldChain: []*header{
							mock(0x2).Parent(0x1).Diff(3),
						},
					},
				},
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
			History: []*headerEvnt{
				{
					header: mock(0x0),
				},
				{
					header: mock(0x1),
					event: &evnt{
						NewChain: []*header{
							mock(0x1),
						},
						Diff: big.NewInt(2),
					},
				},
				{
					header: mock(0x2),
					event: &evnt{
						NewChain: []*header{
							mock(0x2),
						},
						Diff: big.NewInt(4),
					},
				},
				{
					header: mock(0x3),
					event: &evnt{
						NewChain: []*header{
							mock(0x3),
						},
						Diff: big.NewInt(7),
					},
				},
				{
					// First reorg
					header: mock(0x4).Parent(0x1).Diff(10).Number(2),
					event: &evnt{
						// add block 4
						NewChain: []*header{
							mock(0x4).Parent(0x1).Diff(10).Number(2),
						},
						// remove block 2 and 3
						OldChain: []*header{
							mock(0x2),
							mock(0x3),
						},
						Diff: big.NewInt(12),
					},
				},
				{
					header: mock(0x5).Parent(0x4).Diff(11).Number(3),
					event: &evnt{
						NewChain: []*header{
							mock(0x5).Parent(0x4).Diff(11).Number(3),
						},
						Diff: big.NewInt(23),
					},
				},
				{
					header: mock(0x6).Parent(0x3).Number(4),
					event: &evnt{
						// lower difficulty, its a fork
						OldChain: []*header{
							mock(0x6).Parent(0x3).Number(4),
						},
					},
				},
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
			History: []*headerEvnt{
				{
					header: mock(0x0),
				},
				{
					header: mock(0x1),
					event: &evnt{
						NewChain: []*header{
							mock(0x1),
						},
						Diff: big.NewInt(2),
					},
				},
				{
					header: mock(0x2),
					event: &evnt{
						NewChain: []*header{
							mock(0x2),
						},
						Diff: big.NewInt(4),
					},
				},
				{
					header: mock(0x3),
					event: &evnt{
						NewChain: []*header{
							mock(0x3),
						},
						Diff: big.NewInt(7),
					},
				},
				{
					// fork 1. 0x1 -> 0x2 -> 0x4
					header: mock(0x4).Parent(0x2).Diff(11),
					event: &evnt{
						NewChain: []*header{
							mock(0x4).Parent(0x2).Diff(11),
						},
						OldChain: []*header{
							mock(0x3),
						},
						Diff: big.NewInt(15),
					},
				},
				{
					// fork 2. 0x1 -> 0x2 -> 0x3 -> 0x5
					header: mock(0x5).Parent(0x3),
					event: &evnt{
						OldChain: []*header{
							mock(0x5).Parent(0x3),
						},
					},
				},
				{
					// fork 3. 0x1 -> 0x2 -> 0x6
					header: mock(0x6).Parent(0x2).Diff(5),
					event: &evnt{
						OldChain: []*header{
							mock(0x6).Parent(0x2).Diff(5),
						},
					},
				},
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
		{
			Name: "Head from old long fork",
			History: []*headerEvnt{
				{
					header: mock(0x0),
				},
				{
					header: mock(0x1),
					event: &evnt{
						NewChain: []*header{
							mock(0x1),
						},
						Diff: big.NewInt(2),
					},
				},
				{
					header: mock(0x2),
					event: &evnt{
						NewChain: []*header{
							mock(0x2),
						},
						Diff: big.NewInt(4),
					},
				},
				{
					// fork 1.
					header: mock(0x3).Parent(0x0).Diff(5),
					event: &evnt{
						NewChain: []*header{
							mock(0x3).Parent(0x0).Diff(5),
						},
						OldChain: []*header{
							mock(0x1),
							mock(0x2),
						},
						Diff: big.NewInt(6),
					},
				},
				{
					// Add back the 0x2 fork
					header: mock(0x4).Parent(0x2).Diff(10),
					event: &evnt{
						NewChain: []*header{
							mock(0x4).Parent(0x2).Diff(10),
							mock(0x2),
							mock(0x1),
						},
						OldChain: []*header{
							mock(0x3).Parent(0x0).Diff(5),
						},
						Diff: big.NewInt(14),
					},
				},
			},
			Head: mock(0x4).Parent(0x2).Diff(10),
			Forks: []*header{
				mock(0x2),
				mock(0x3).Parent(0x0).Diff(5),
			},
			Chain: []*header{
				mock(0x0),
				mock(0x1),
				mock(0x2),
				mock(0x4).Parent(0x2).Diff(10),
			},
			TD: 1 + 1 + 2 + 10,
		},
	}

	for _, cc := range cases {
		t.Run(cc.Name, func(t *testing.T) {
			b := NewTestBlockchain(t, nil)

			chain := dummyChain{
				headers: map[byte]*types.Header{},
			}
			for _, i := range cc.History {
				if err := chain.add(i.header); err != nil {
					t.Fatal(err)
				}
			}

			checkEvents := func(a []*header, b []*types.Header) {
				if len(a) != len(b) {
					t.Fatal("bad size")
				}
				for indx := range a {
					if chain.headers[a[indx].hash].Hash != b[indx].Hash {
						t.Fatal("bad")
					}
				}
			}

			// genesis is 0x0
			if err := b.WriteHeaderGenesis(chain.headers[0x0]); err != nil {
				t.Fatal(err)
			}

			// we need to subscribe just after the genesis and history
			sub := b.SubscribeEvents()

			// run the history
			for i := 1; i < len(cc.History); i++ {
				if err := b.WriteHeaders([]*types.Header{chain.headers[cc.History[i].header.hash]}); err != nil {
					t.Fatal(err)
				}

				// get the event
				evnt := sub.GetEvent()
				checkEvents(cc.History[i].event.NewChain, evnt.NewChain)
				checkEvents(cc.History[i].event.OldChain, evnt.OldChain)

				if evnt.Difficulty != nil {
					if evnt.Difficulty.Cmp(cc.History[i].event.Diff) != 0 {
						t.Fatal("bad diff in event")
					}
				}
			}

			head := b.Header()

			expected, ok := chain.headers[cc.Head.hash]
			assert.True(t, ok)

			// check that we got the right hash
			assert.Equal(t, head.Hash, expected.Hash)

			forks, err := b.GetForks()
			assert.NoError(t, err)

			expectedForks := []types.Hash{}

			for _, i := range cc.Forks {
				expectedForks = append(expectedForks, chain.headers[i.hash].Hash)
			}

			if len(forks) != 0 {
				if len(forks) != len(expectedForks) {
					t.Fatalf("forks length dont match, expected %d but found %d", len(expectedForks), len(forks))
				} else {
					if !reflect.DeepEqual(forks, expectedForks) {
						t.Fatal("forks dont match")
					}
				}
			}

			// Check chain of forks
			if cc.Chain != nil {
				for indx, i := range cc.Chain {
					block, _ := b.GetBlockByNumber(uint64(indx), true)
					if block.Hash().String() != chain.headers[i.hash].Hash.String() {
						t.Fatal("bad")
					}
				}
			}

			if td, _ := b.GetChainTD(); cc.TD != td.Uint64() {
				t.Fatal("bad")
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
	assert.Error(t, b.WriteHeadersWithBodies([]*types.Header{h1[12]}))
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
		i, err := b.db.ReadBody(block.Hash())
		assert.NoError(t, err)

		assert.Len(t, i.Transactions, 1)
		assert.Equal(t, i.Transactions[0].Nonce, block.Number())

		// check receipts
		r, _ := b.db.ReadReceipts(block.Hash())
		assert.Len(t, r, 1)
	}
}
