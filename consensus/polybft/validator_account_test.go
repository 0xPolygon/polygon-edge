package polybft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestAccountSet_GetAddresses(t *testing.T) {
	t.Parallel()

	address1, address2, address3 := types.Address{4, 3}, types.Address{68, 123}, types.Address{168, 123}
	ac := AccountSet{
		&ValidatorMetadata{Address: address1},
		&ValidatorMetadata{Address: address2},
		&ValidatorMetadata{Address: address3},
	}
	rs := ac.GetAddresses()
	assert.Len(t, rs, 3)
	assert.Equal(t, address1, rs[0])
	assert.Equal(t, address2, rs[1])
	assert.Equal(t, address3, rs[2])
}

func TestAccountSet_GetBlsKeys(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(3)
	assert.NoError(t, err)

	key1, key2, key3 := keys[0], keys[1], keys[2]
	ac := AccountSet{
		&ValidatorMetadata{BlsKey: key1.PublicKey()},
		&ValidatorMetadata{BlsKey: key2.PublicKey()},
		&ValidatorMetadata{BlsKey: key3.PublicKey()},
	}
	rs := ac.GetBlsKeys()
	assert.Len(t, rs, 3)
	assert.Equal(t, key1.PublicKey(), rs[0])
	assert.Equal(t, key2.PublicKey(), rs[1])
	assert.Equal(t, key3.PublicKey(), rs[2])
}

func TestAccountSet_IndexContainsAddressesAndContainsNodeId(t *testing.T) {
	t.Parallel()

	const count = 10

	dummy := types.Address{2, 3, 4}
	validators := newTestValidators(count).getPublicIdentities()
	addresses := [count]types.Address{}

	for i, validator := range validators {
		addresses[i] = validator.Address
	}

	for i, a := range addresses {
		assert.Equal(t, i, validators.Index(a))
		assert.True(t, validators.ContainsAddress(a))
		assert.True(t, validators.ContainsNodeID(a.String()))
	}

	assert.Equal(t, -1, validators.Index(dummy))
	assert.False(t, validators.ContainsAddress(dummy))
	assert.False(t, validators.ContainsNodeID(dummy.String()))
}

func TestAccountSet_Len(t *testing.T) {
	t.Parallel()

	const count = 10

	ac := AccountSet{}

	for i := 0; i < count; i++ {
		ac = append(ac, &ValidatorMetadata{})
		assert.Equal(t, i+1, ac.Len())
	}
}

func TestAccountSet_ApplyDelta(t *testing.T) {
	t.Parallel()

	// Add a couple of validators to the snapshot => validators are present in the snapshot after applying such delta
	vals := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})

	type Step struct {
		added  []string
		remove []uint64
		expect []string
	}

	cases := []struct {
		name  string
		steps []*Step
	}{
		{
			name: "Basic",
			steps: []*Step{
				{
					[]string{"A", "B", "C", "D"},
					nil,
					[]string{"A", "B", "C", "D"},
				},
				{
					// add two new items and remove 3 (one does not exists)
					[]string{"E", "F"},
					[]uint64{1, 2, 5},
					[]string{"A", "D", "E", "F"},
				},
			},
		},
		{
			name: "AddRemoveSameValidator",
			steps: []*Step{
				{
					[]string{"A"},
					[]uint64{0},
					[]string{"A"},
				},
			},
		},
	}

	for _, cc := range cases {
		snapshot := AccountSet{}

		t.Run(cc.name, func(t *testing.T) {
			t.Parallel()

			for _, step := range cc.steps {
				delta := &ValidatorSetDelta{
					Added:   vals.getPublicIdentities(step.added...),
					Removed: bitmap.Bitmap{},
				}
				for _, i := range step.remove {
					delta.Removed.Set(i)
				}

				// apply delta
				var err error
				snapshot, err = snapshot.ApplyDelta(delta)
				assert.NoError(t, err)

				// validate validator set
				if len(step.expect) != snapshot.Len() {
					t.Fatal("incorrect length")
				}
				for _, acct := range step.expect {
					v := vals.getValidator(acct)
					if !snapshot.ContainsAddress(v.ValidatorMetadata().Address) {
						t.Fatalf("not found '%s'", acct)
					}
				}
			}
		})
	}
}
