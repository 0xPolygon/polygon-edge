package network

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/0xPolygon/polygon-sdk/secrets/local"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
)

var initialPort = uint64(2000)

func CreateServer(t *testing.T, callback func(c *Config)) *Server {
	// create the server
	cfg := DefaultConfig()
	cfg.Addr.Port = int(atomic.AddUint64(&initialPort, 1))
	cfg.Chain = &chain.Chain{
		Params: &chain.Params{
			ChainID: 1,
		},
	}

	logger := hclog.NewNullLogger()

	if callback != nil {
		callback(cfg)
	}

	secretsManager, factoryErr := local.SecretsManagerFactory(
		nil,
		&secrets.SecretsManagerParams{
			Logger: logger,
			Extra: map[string]interface{}{
				secrets.Path: cfg.DataDir,
			},
		},
	)

	assert.NoError(t, factoryErr)

	cfg.SecretsManager = secretsManager

	srv, err := NewServer(logger, cfg)
	assert.NoError(t, err)

	return srv
}

func MultiJoinSerial(t *testing.T, srvs []*Server) {
	dials := []*Server{}
	for i := 0; i < len(srvs)-1; i++ {
		srv, dst := srvs[i], srvs[i+1]
		dials = append(dials, srv, dst)
	}
	MultiJoin(t, dials...)
}

func MultiJoin(t *testing.T, srvs ...*Server) {
	if len(srvs)%2 != 0 {
		t.Fatal("not an even number")
	}

	doneCh := make(chan error)
	for i := 0; i < len(srvs); i += 2 {
		go func(i int) {
			src, dst := srvs[i], srvs[i+1]
			doneCh <- src.Join(dst.AddrInfo(), 10*time.Second)
		}(i)
	}

	for i := 0; i < len(srvs)/2; i++ {
		err := <-doneCh
		if err != nil {
			t.Fatal(err)
		}
	}
}

func getTestConfig(callback func(c *Config)) *Config {
	cfg := DefaultConfig()
	cfg.Addr.Port = int(atomic.AddUint64(&initialPort, 1))
	cfg.Chain = &chain.Chain{
		Params: &chain.Params{
			ChainID: 1,
		},
	}

	logger := hclog.NewNullLogger()

	if callback != nil {
		callback(cfg)
	}

	secretsManager, _ := local.SecretsManagerFactory(
		nil,
		&secrets.SecretsManagerParams{
			Logger: logger,
			Extra: map[string]interface{}{
				secrets.Path: cfg.DataDir,
			},
		},
	)

	cfg.SecretsManager = secretsManager

	return cfg
}
func GenerateTestMultiAddr(t *testing.T) multiaddr.Multiaddr {
	libp2pKey, _, keyErr := GenerateAndEncodeLibp2pKey()
	if keyErr != nil {
		t.Fatalf("unable to generate libp2p key, %v", keyErr)
	}
	nodeId, err := peer.IDFromPrivateKey(libp2pKey)
	assert.NoError(t, err)
	rand.Seed(time.Now().Unix())
	randomPort := rand.Intn(10) + 10010
	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", randomPort, nodeId))
	assert.NoError(t, err)
	return addr
}
func GenerateTestLibp2pKey(t *testing.T) (crypto.PrivKey, string) {
	t.Helper()

	dir, err := ioutil.TempDir(os.TempDir(), "")
	assert.NoError(t, err)

	// Instantiate the correct folder structure
	setupErr := common.SetupDataDir(dir, []string{"libp2p"})
	if setupErr != nil {
		t.Fatalf("unable to generate libp2p folder structure, %v", setupErr)
	}

	localSecretsManager, factoryErr := local.SecretsManagerFactory(
		nil,
		&secrets.SecretsManagerParams{
			Logger: hclog.NewNullLogger(),
			Extra: map[string]interface{}{
				secrets.Path: dir,
			},
		})
	assert.NoError(t, factoryErr)

	libp2pKey, libp2pKeyEncoded, keyErr := GenerateAndEncodeLibp2pKey()
	if keyErr != nil {
		t.Fatalf("unable to generate libp2p key, %v", keyErr)
	}

	if setErr := localSecretsManager.SetSecret(secrets.NetworkKey, libp2pKeyEncoded); setErr != nil {
		t.Fatalf("unable to save libp2p key, %v", setErr)
	}

	t.Cleanup(func() {
		// remove directory after test is done
		assert.NoError(t, os.RemoveAll(dir))
	})

	return libp2pKey, dir
}
