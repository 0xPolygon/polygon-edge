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

	correctValset := pool.ValidatorSet()

	h := &types.Header{}

	signerA := signer.NewSigner(signer.NewECDSAKeyManagerFromKey(pool.get("A").priv))

	err := signerA.InitIBFTExtra(h, nil, correctValset)
	assert.NoError(t, err)

	// non-validator address
	pool.add("X")

	signerX := signer.NewSigner(
		signer.NewECDSAKeyManagerFromKey(pool.get("X").priv),
	)

	badSealedBlock, _ := signerX.WriteProposerSeal(h)
	assert.Error(t, verifyProposerSeal(badSealedBlock, signerA, correctValset))

	// seal the block with a validator
	goodSealedBlock, _ := signerA.WriteProposerSeal(h)
	assert.NoError(t, verifyProposerSeal(goodSealedBlock, signerA, correctValset))
}

func TestSign_CommittedSeals(t *testing.T) {
	t.Parallel()

	pool := newTesterAccountPool(t)
	pool.add("A", "B", "C", "D", "E")

	h := &types.Header{}

	correctValSet := pool.ValidatorSet()

	signerA := signer.NewSigner(
		signer.NewECDSAKeyManagerFromKey(pool.get("A").priv),
	)
	err := signerA.InitIBFTExtra(h, &types.Header{}, correctValSet)
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

		return signerA.VerifyCommittedSeals(sealed, correctValSet, OptimalQuorumSize(correctValSet))
	}

	// Correct
	assert.NoError(t, buildCommittedSeal([]string{"A", "B", "C", "D"}))

	// Failed - Non validator signature
	assert.Error(t, buildCommittedSeal([]string{"A", "X"}))

	// Failed - Not enough signatures
	assert.Error(t, buildCommittedSeal([]string{"A"}))
}
