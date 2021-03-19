package ibft

import (
	"crypto/ecdsa"
	"testing"

	"github.com/0xPolygon/minimal/consensus/ibft/aux"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

type validatorsPool struct {
	accounts map[string]*ecdsa.PrivateKey
}

func (v *validatorsPool) list() (res []types.Address) {
	res = []types.Address{}
	for _, priv := range v.accounts {
		res = append(res, crypto.PubKeyToAddress(&priv.PublicKey))
	}
	return res
}

func (v *validatorsPool) address(name string) types.Address {
	return crypto.PubKeyToAddress(&(v.accounts[name]).PublicKey)
}

func (v *validatorsPool) sign(name string, h *types.Header) *types.Header {
	h, _ = writeSeal(v.accounts[name], h)
	return h
}

func (v *validatorsPool) genesis() *types.Header {
	genesis := &types.Header{
		MixHash: types.IstanbulDigest,
	}
	putIbftExtraValidators(genesis, v.list())
	genesis.ComputeHash()
	return genesis
}

func (v *validatorsPool) add(accounts ...string) {
	if v.accounts == nil {
		v.accounts = map[string]*ecdsa.PrivateKey{}
	}
	for _, account := range accounts {
		priv, err := crypto.GenerateKey()
		if err != nil {
			panic("BUG: Failed to generate crypto key")
		}
		v.accounts[account] = priv
	}
}

func TestSnapshotVerifyHeader(t *testing.T) {
	pool := &validatorsPool{}
	pool.add("A", "B", "C")

	store := aux.NewMockBlockchain()
	b := &snapshotState{
		store: store,
	}
	genesis := pool.genesis()
	store.Append(genesis)
	assert.NoError(t, b.setup())

	h := &types.Header{
		Number:    10,
		ExtraData: genesis.ExtraData,
	}
	h = pool.sign("A", h)
	h.ComputeHash()

	if err := b.verifyHeader(h); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotVoting(t *testing.T) {
	// each mockVote will be an independent block
	type mockVote struct {
		validator string
		voted     string
		auth      bool
	}

	var cases = []struct {
		validators []string
		votes      []mockVote
		results    []string
	}{
		{
			validators: []string{"A"},
			votes: []mockVote{
				{validator: "A", voted: "B", auth: true},
				{validator: "B"},
				{validator: "A", voted: "C", auth: true},
			},
			results: []string{"A", "B"},
		},
	}
	for _, c := range cases {
		pool := &validatorsPool{}
		pool.add(c.validators...)

		store := aux.NewMockBlockchain()
		b := &snapshotState{
			store: store,
		}

		// create genesis block
		genesis := pool.genesis()
		store.Append(genesis)

		// start the snapshot state after genesis is done
		assert.NoError(t, b.setup())

		// create votes
		parentHash := types.Hash{}

		headers := []*types.Header{}
		for num, v := range c.votes {
			pool.add(v.validator, v.voted)

			h := &types.Header{
				Number:     uint64(num + 1),
				ParentHash: parentHash,
				Miner:      pool.address(v.voted),
				MixHash:    types.IstanbulDigest,
				ExtraData:  genesis.ExtraData,
			}
			if v.auth {
				// add auth to the vote
				h.Nonce.Set(nonceAuthVote)
			}

			// sign the vote
			h = pool.sign(v.validator, h)

			h.ComputeHash()
			store.Append(h)

			parentHash = h.Hash
			headers = append(headers, h)
		}

		// process all the headers at the same time
		if err := b.processHeaders(headers); err != nil {
			t.Fatal(err)
		}
	}
}
