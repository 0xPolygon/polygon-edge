package ibft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestSign_Sealer(t *testing.T) {
	pool := newTesterAccountPool(t)
	pool.add("A")

	snap := &Snapshot{
		Set: pool.ValidatorSet(),
	}

	h := &types.Header{}
	signerA := signer.NewSigner(signer.NewECDSAKeyManagerFromKey(pool.get("A").priv))

	err := signerA.InitIBFTExtra(h, &types.Header{}, pool.ValidatorSet())
	assert.NoError(t, err)

	// non-validator address
	pool.add("X")

	signerX := signer.NewSigner(signer.NewECDSAKeyManagerFromKey(pool.get("X").priv))

	badSealedHeader, _ := signerX.WriteSeal(h)

	signerBySeal1, err := signerA.EcrecoverFromHeader(badSealedHeader)
	assert.NoError(t, err)
	assert.False(t, snap.Set.Includes(signerBySeal1), "signer shouldn't exist")

	// seal the block with a validator
	goodSealedHeader, _ := signerA.WriteSeal(h)

	signerBySeal2, err := signerA.EcrecoverFromHeader(goodSealedHeader)
	assert.NoError(t, err)
	assert.True(t, snap.Set.Includes(signerBySeal2), "signer shouldn't exist")
}

func TestSign_CommittedSeals(t *testing.T) {
	pool := newTesterAccountPool(t)
	pool.add("A", "B", "C", "D", "E")

	snap := &Snapshot{
		Set: pool.ValidatorSet(),
	}

	h := &types.Header{}

	signerA := signer.NewSigner(signer.NewECDSAKeyManagerFromKey(pool.get("A").priv))
	err := signerA.InitIBFTExtra(h, &types.Header{}, pool.ValidatorSet())
	assert.NoError(t, err)

	// non-validator address
	pool.add("X")

	buildCommittedSeal := func(names []string) error {
		seals := map[types.Address][]byte{}

		for _, name := range names {
			account := pool.get(name)
			signer := signer.NewSigner(signer.NewECDSAKeyManagerFromKey(account.priv))
			seal, err := signer.CreateCommittedSeal(h)

			assert.NoError(t, err)

			seals[account.Address()] = seal
		}

		sealed, err := signerA.WriteCommittedSeals(h, seals)

		assert.NoError(t, err)

		return signerA.VerifyCommittedSeal(snap.Set, sealed, OptimalQuorumSize(snap.Set))
	}

	// Correct
	assert.NoError(t, buildCommittedSeal([]string{"A", "B", "C", "D"}))

	// // Failed - Repeated signature
	assert.Error(t, buildCommittedSeal([]string{"A", "A"}))

	// Failed - Non validator signature
	assert.Error(t, buildCommittedSeal([]string{"A", "X"}))

	// Failed - Not enough signatures
	assert.Error(t, buildCommittedSeal([]string{"A"}))
}

func TestSign_Messages(t *testing.T) {
	pool := newTesterAccountPool(t)
	pool.add("A")

	msg := &proto.MessageReq{}
	signerA := signer.NewECDSAKeyManagerFromKey(pool.get("A").priv)
	assert.NoError(t, signerA.SignIBFTMessage(msg))
	assert.NoError(t, signerA.ValidateIBFTMessage(msg))

	assert.Equal(t, msg.From, pool.get("A").Address().String())
}
