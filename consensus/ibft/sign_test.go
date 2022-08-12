package ibft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestSign_Sealer(t *testing.T) {
	t.Parallel()

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

	signerX := signer.NewSigner(
		signer.NewECDSAKeyManagerFromKey(pool.get("X").priv),
		signer.NewECDSAKeyManagerFromKey(pool.get("X").priv),
	)

	badSealedBlock, _ := signerX.WriteProposerSeal(h)
	assert.Error(t, verifySigner(signerA, snap.Set, badSealedBlock))

	// seal the block with a validator
	goodSealedBlock, _ := signerA.WriteProposerSeal(h)
	assert.NoError(t, verifySigner(signerA, snap.Set, goodSealedBlock))
}

func TestSign_CommittedSeals(t *testing.T) {
	t.Parallel()

	pool := newTesterAccountPool(t)
	pool.add("A", "B", "C", "D", "E")

	snap := &Snapshot{
		Set: pool.ValidatorSet(),
	}

	h := &types.Header{}

	signerA := signer.NewSigner(
		signer.NewECDSAKeyManagerFromKey(pool.get("A").priv),
		signer.NewECDSAKeyManagerFromKey(pool.get("A").priv),
	)
	err := signerA.InitIBFTExtra(h, &types.Header{}, pool.ValidatorSet())
	assert.NoError(t, err)

	h.Hash, err = signerA.CalculateHeaderHash(h)
	if err != nil {
		t.Fatalf("Unable to calculate hash, %v", err)
	}

	// non-validator address
	pool.add("X")

	buildCommittedSeal := func(names []string) error {
		seals := map[types.Address][]byte{}

		for _, name := range names {
			acc := pool.get(name)

			signer := signer.NewSigner(
				signer.NewECDSAKeyManagerFromKey(
					acc.priv,
				),
			)
			seal, err := signer.CreateCommittedSeal(h.Hash.Bytes())

			assert.NoError(t, err)

			seals[acc.Address()] = seal
		}

		sealed, err := signerA.WriteCommittedSeals(h, seals)

		assert.NoError(t, err)

		return signerA.VerifyCommittedSeals(snap.Set, sealed, OptimalQuorumSize(snap.Set))
	}

	// Correct
	assert.NoError(t, buildCommittedSeal([]string{"A", "B", "C", "D"}))

	// Failed - Non validator signature
	assert.Error(t, buildCommittedSeal([]string{"A", "X"}))

	// Failed - Not enough signatures
	assert.Error(t, buildCommittedSeal([]string{"A"}))
}
