package ibft

import (
	"context"
	"testing"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestOperator_GetNextCandidate(t *testing.T) {
	// we cannot vote if there is already a pending vote for our proposal
	pool := newTesterAccountPool()
	pool.add("A", "B", "C")

	ibft := &Ibft{
		validatorKeyAddr: pool.get("A").Address(),
	}

	snap := &Snapshot{
		Set: pool.ValidatorSet(),
		Votes: []*Vote{
			{
				Validator: pool.get("A").Address(),
				Address:   pool.get("B").Address(),
			},
		},
	}

	o := &operator{
		ibft: ibft,
		candidates: []*proto.Candidate{
			{
				Address: pool.get("B").Address().String(),
				Auth:    false,
			},
		},
	}

	// it has already voted once, it cannot vote again
	assert.Nil(t, o.getNextCandidate(snap))

	snap.Votes = nil

	// there are no votes so it can vote
	assert.NotNil(t, o.getNextCandidate(snap))

	snap.Set = []types.Address{}

	// it was a removal and since the candidate is not on the set anymore
	// is removed from the candidates list
	assert.Nil(t, o.getNextCandidate(snap))
	assert.Len(t, o.candidates, 0)

	// Try to insert now a new candidate
	o.candidates = []*proto.Candidate{
		{
			Address: pool.get("B").Address().String(),
			Auth:    true,
		},
	}

	assert.NotNil(t, o.getNextCandidate(snap))

	// add the new candidate to the set
	snap.Set = pool.ValidatorSet()

	// now the candidate is on the new set so we have to remove
	// the candidate
	assert.Nil(t, o.getNextCandidate(snap))
	assert.Len(t, o.candidates, 0)
}

func TestOperator_Propose(t *testing.T) {
	pool := newTesterAccountPool()
	pool.add("A", "B", "C")

	ibft := &Ibft{
		blockchain: blockchain.TestBlockchain(t, pool.genesis()),
		config:     &consensus.Config{},
		epochSize:  DefaultEpochSize,
	}
	assert.NoError(t, ibft.setupSnapshot())

	o := &operator{ibft: ibft}

	pool.add("X")

	// we cannot propose to add a validator already in the set
	_, err := o.Propose(context.Background(), &proto.Candidate{
		Address: pool.get("A").Address().String(),
		Auth:    true,
	})
	assert.Error(t, err)

	// we cannot propose remove a validator that is not part of the set
	_, err = o.Propose(context.Background(), &proto.Candidate{
		Address: pool.get("X").Address().String(),
		Auth:    false,
	})
	assert.Error(t, err)

	// we can send either add or del proposals
	_, err = o.Propose(context.Background(), &proto.Candidate{
		Address: pool.get("X").Address().String(),
		Auth:    true,
	})
	assert.NoError(t, err)
	assert.Len(t, o.candidates, 1)

	_, err = o.Propose(context.Background(), &proto.Candidate{
		Address: pool.get("A").Address().String(),
		Auth:    false,
	})
	assert.NoError(t, err)
	assert.Len(t, o.candidates, 2)

	// we cannot send the same proposal twice
	_, err = o.Propose(context.Background(), &proto.Candidate{
		Address: pool.get("A").Address().String(),
		Auth:    false,
	})
	assert.Error(t, err)
}
