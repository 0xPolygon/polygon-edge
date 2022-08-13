package ibft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestSign_Sealer(t *testing.T) {
	t.Parallel()

	pool := newTesterAccountPool()
	pool.add("A")

	snap := &Snapshot{
		Set: pool.ValidatorSet(),
	}

	h := &types.Header{}
	putIbftExtraValidators(h, pool.ValidatorSet())

	// non-validator address
	pool.add("X")

	badSealedBlock, _ := writeProposerSeal(pool.get("X").priv, h)
	assert.Error(t, verifySigner(snap, badSealedBlock))

	// seal the block with a validator
	goodSealedBlock, _ := writeProposerSeal(pool.get("A").priv, h)
	assert.NoError(t, verifySigner(snap, goodSealedBlock))
}

func TestSign_CommittedSeals(t *testing.T) {
	t.Parallel()

	pool := newTesterAccountPool()
	pool.add("A", "B", "C", "D", "E")

	snap := &Snapshot{
		Set: pool.ValidatorSet(),
	}

	h := &types.Header{
		ExtraData: []byte{},
	}

	putIbftExtraValidators(h, pool.ValidatorSet())

	hash, err := calculateHeaderHash(h)
	if err != nil {
		t.Fatalf("Unable to calculate hash, %v", err)
	}

	h.Hash = types.BytesToHash(hash)

	// non-validator address
	pool.add("X")

	buildCommittedSeal := func(accnt []string) error {
		seals := [][]byte{}

		for _, accnt := range accnt {
			seal, err := writeCommittedSeal(pool.get(accnt).priv, h.Hash.Bytes())

			assert.NoError(t, err)

			seals = append(seals, seal)
		}

		sealed, err := writeCommittedSeals(h, seals)

		assert.NoError(t, err)

		return verifyCommittedFields(snap, sealed, OptimalQuorumSize)
	}

	// Correct
	assert.NoError(t, buildCommittedSeal([]string{"A", "B", "C", "D"}))

	// Failed - Repeated signature
	assert.Error(t, buildCommittedSeal([]string{"A", "A"}))

	// Failed - Non validator signature
	assert.Error(t, buildCommittedSeal([]string{"A", "X"}))

	// Failed - Not enough signatures
	assert.Error(t, buildCommittedSeal([]string{"A"}))
}
