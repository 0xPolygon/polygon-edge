package ibft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestSign_Sealer(t *testing.T) {
	pool := newTesterAccountPool()
	pool.add("A")

	snap := &Snapshot{
		Set: pool.ValidatorSet(),
	}

	h := &types.Header{}
	putIbftExtraValidators(h, pool.ValidatorSet())

	// non-validator address
	pool.add("X")

	badSealedBlock, _ := writeSeal(pool.get("X").priv, h)
	assert.Error(t, verifySigner(snap, badSealedBlock))

	// seal the block with a validator
	goodSealedBlock, _ := writeSeal(pool.get("A").priv, h)
	assert.NoError(t, verifySigner(snap, goodSealedBlock))
}

func TestSign_CommittedSeals(t *testing.T) {
	pool := newTesterAccountPool()
	pool.add("A", "B", "C", "D", "E")

	snap := &Snapshot{
		Set: pool.ValidatorSet(),
	}

	h := &types.Header{}
	putIbftExtraValidators(h, pool.ValidatorSet())

	// non-validator address
	pool.add("X")

	buildCommittedSeal := func(accnt []string) error {
		seals := [][]byte{}

		for _, accnt := range accnt {
			seal, err := writeCommittedSeal(pool.get(accnt).priv, h)

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

func TestSign_Messages(t *testing.T) {
	pool := newTesterAccountPool()
	pool.add("A")

	msg := &proto.MessageReq{}
	assert.NoError(t, signMsg(pool.get("A").priv, msg))
	assert.NoError(t, validateMsg(msg))

	assert.Equal(t, msg.From, pool.get("A").Address().String())
}
