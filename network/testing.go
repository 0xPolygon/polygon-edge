package network

import (
	"fmt"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"io/ioutil"
	"os"
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

func waitForMesh() {
	// wait until gossip protocol builds the mesh network (https://github.com/libp2p/specs/blob/master/pubsub/gossipsub/gossipsub-v1.0.md)
	waitForMeshCustom(time.Second * 2)
}

func waitForMeshCustom(duration time.Duration) {
	time.Sleep(duration)
}

func createServers(
	count int,
	params []*CreateServerParams,
) ([]*Server, error) {
	servers := make([]*Server, count)

	for i := 0; i < count; i++ {
		server, createErr := CreateServer(params[i])
		if createErr != nil {
			return nil, createErr
		}

		servers[i] = server
	}

	return servers, nil
}

type CreateServerParams struct {
	ConfigCallback func(c *Config)      // Additional logic that needs to be executed on the configuration
	ServerCallback func(server *Server) // Additional logic that needs to be executed on the server before starting
	Logger         hclog.Logger
}

var (
	emptyParams = &CreateServerParams{}
)

func CreateServer(params *CreateServerParams) (*Server, error) {
	cfg := DefaultConfig()
	port, portErr := tests.GetFreePort()
	if portErr != nil {
		return nil, fmt.Errorf("unable to fetch free port, %v", portErr)
	}
	cfg.Addr.Port = port
	cfg.Chain = &chain.Chain{
		Params: &chain.Params{
			ChainID: 1,
		},
	}

	if params == nil {
		params = emptyParams
	}

	if params.ConfigCallback != nil {
		params.ConfigCallback(cfg)
	}

	if params.Logger == nil {
		params.Logger = hclog.NewNullLogger()
	}

	secretsManager, factoryErr := local.SecretsManagerFactory(
		nil,
		&secrets.SecretsManagerParams{
			Logger: params.Logger,
			Extra: map[string]interface{}{
				secrets.Path: cfg.DataDir,
			},
		},
	)
	if factoryErr != nil {
		return nil, factoryErr
	}

	cfg.SecretsManager = secretsManager

	server, err := NewServer(params.Logger, cfg)
	if err != nil {
		return nil, err
	}

	if params.ServerCallback != nil {
		params.ServerCallback(server)
	}

	startErr := server.Start()

	return server, startErr
}

func MultiJoinSerial(t *testing.T, servers []*Server) {
	dials := make([]*Server, len(servers))
	for i := 0; i < len(servers)-1; i++ {
		src, dst := servers[i], servers[i+1]
		dials = append(dials, src, dst)
	}

	MultiJoin(t, dials...)
}

func MultiJoin(t *testing.T, servers ...*Server) {
	numServers := len(servers)

	if numServers%2 != 0 {
		t.Fatal("uneven number of servers passed in")
	}

	doneCh := make(chan error)
	for i := 0; i < numServers; i += 2 {
		go func(i int) {
			src, dst := servers[i], servers[i+1]
			doneCh <- src.Join(dst.AddrInfo(), 10*time.Second)
		}(i)
	}

	for i := 0; i < numServers/2; i++ {
		err := <-doneCh
		if err != nil {
			t.Fatalf("Unable to complete join procedure, %v", err)
		}
	}
}

func GenerateTestMultiAddr(t *testing.T) multiaddr.Multiaddr {
	libp2pKey, _, keyErr := GenerateAndEncodeLibp2pKey()
	if keyErr != nil {
		t.Fatalf("unable to generate libp2p key, %v", keyErr)
	}

	nodeId, err := peer.IDFromPrivateKey(libp2pKey)
	assert.NoError(t, err)

	port, portErr := tests.GetFreePort()
	if portErr != nil {
		t.Fatalf("Unable to fetch free port, %v", portErr)
	}

	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", port, nodeId))
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
		_ = os.RemoveAll(dir)
	})

	return libp2pKey, dir
}
