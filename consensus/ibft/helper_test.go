package ibft

import (
	"crypto/ecdsa"
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/stretchr/testify/assert"
)

type testerAccount struct {
	alias string
	priv  *ecdsa.PrivateKey
}

func (t *testerAccount) Address() types.Address {
	return crypto.PubKeyToAddress(&t.priv.PublicKey)
}

func (t *testerAccount) sign(h *types.Header) (*types.Header, error) {
	signer := signer.NewSigner(
		signer.NewECDSAKeyManagerFromKey(t.priv),
	)

	return signer.WriteSeal(h)
}

type testerAccountPool struct {
	t        *testing.T
	accounts []*testerAccount
}

func newTesterAccountPool(t *testing.T, num ...int) *testerAccountPool {
	t.Helper()

	pool := &testerAccountPool{
		t:        t,
		accounts: []*testerAccount{},
	}

	if len(num) == 1 {
		for i := 0; i < num[0]; i++ {
			key, _ := crypto.GenerateECDSAKey()

			pool.accounts = append(pool.accounts, &testerAccount{
				alias: strconv.Itoa(i),
				priv:  key,
			})
		}
	}

	return pool
}

func (ap *testerAccountPool) add(accounts ...string) {
	ap.t.Helper()

	for _, account := range accounts {
		if acct := ap.get(account); acct != nil {
			continue
		}

		priv, err := crypto.GenerateECDSAKey()
		if err != nil {
			panic("BUG: Failed to generate crypto key")
		}

		ap.accounts = append(ap.accounts, &testerAccount{
			alias: account,
			priv:  priv,
		})
	}
}

func (ap *testerAccountPool) genesis() *chain.Genesis {
	ap.t.Helper()

	genesis := &types.Header{
		MixHash: signer.IstanbulDigest,
	}

	signer := signer.NewSigner(
		signer.NewECDSAKeyManagerFromKey(ap.get("A").priv),
	)

	err := signer.InitIBFTExtra(genesis, nil, ap.ValidatorSet())
	assert.NoError(ap.t, err)

	genesis.ComputeHash()

	c := &chain.Genesis{
		Mixhash:   genesis.MixHash,
		ExtraData: genesis.ExtraData,
	}

	return c
}

func (ap *testerAccountPool) get(name string) *testerAccount {
	ap.t.Helper()

	for _, i := range ap.accounts {
		if i.alias == name {
			return i
		}
	}

	return nil
}

func (ap *testerAccountPool) ValidatorSet() validators.Validators {
	ap.t.Helper()

	v := validators.ECDSAValidators{}
	for _, i := range ap.accounts {
		v.Add(&validators.ECDSAValidator{
			Address: i.Address(),
		})
	}

	return &v
}
