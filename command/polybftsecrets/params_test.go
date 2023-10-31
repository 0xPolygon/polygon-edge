package polybftsecrets

import (
	"encoding/hex"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
)

// Test initKeys
func Test_initKeys(t *testing.T) {
	t.Parallel()

	// Creates test directory
	dir, err := os.MkdirTemp("", "test")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	sm, err := helper.SetupLocalSecretsManager(dir)
	require.NoError(t, err)

	ip := &initParams{
		generatesAccount: false,
		generatesNetwork: false,
	}

	_, err = ip.initKeys(sm)
	require.NoError(t, err)

	assert.False(t, fileExists(path.Join(dir, "consensus/validator.key")))
	assert.False(t, fileExists(path.Join(dir, "consensus/validator-bls.key")))
	assert.False(t, fileExists(path.Join(dir, "libp2p/libp2p.key")))

	ip.generatesAccount = true
	res, err := ip.initKeys(sm)
	require.NoError(t, err)
	assert.Len(t, res, 2)

	assert.True(t, fileExists(path.Join(dir, "consensus/validator.key")))
	assert.True(t, fileExists(path.Join(dir, "consensus/validator-bls.key")))
	assert.False(t, fileExists(path.Join(dir, "libp2p/libp2p.key")))

	ip.generatesNetwork = true
	res, err = ip.initKeys(sm)
	require.NoError(t, err)
	assert.Len(t, res, 1)

	assert.True(t, fileExists(path.Join(dir, "libp2p/libp2p.key")))
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}

	return !info.IsDir()
}

func Test_getResult(t *testing.T) {
	t.Parallel()

	// Creates test directory
	dir, err := os.MkdirTemp("", "test")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	sm, err := helper.SetupLocalSecretsManager(dir)
	require.NoError(t, err)

	ip := &initParams{
		generatesAccount: true,
		generatesNetwork: true,
		printPrivateKey:  true,
	}

	_, err = ip.initKeys(sm)
	require.NoError(t, err)

	res, err := ip.getResult(sm, []string{})
	require.NoError(t, err)

	sir := res.(*SecretsInitResult) //nolint:forcetypeassert

	// Test public key serialization
	privKey, err := hex.DecodeString(sir.PrivateKey)
	require.NoError(t, err)
	k, err := wallet.NewWalletFromPrivKey(privKey)
	require.NoError(t, err)

	pubKey := k.Address().String()
	assert.Equal(t, sir.Address.String(), pubKey)

	// Test BLS public key serialization
	blsPrivKey, err := bls.UnmarshalPrivateKey([]byte(sir.BLSPrivateKey))
	require.NoError(t, err)

	blsPubKey := hex.EncodeToString(blsPrivKey.PublicKey().Marshal())
	assert.Equal(t, sir.BLSPubkey, blsPubKey)
}
