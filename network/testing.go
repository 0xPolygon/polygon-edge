package network

import (
	"context"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"io/ioutil"
	"os"
	"sync"
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

// JoinAndWait is a helper method for joining a destination server
// and waiting for the connection to be successful (destination node is a peer of source)
func JoinAndWait(
	source,
	destination *Server,
	connectTimeout time.Duration,
	joinTimeout time.Duration,
) error {
	if joinTimeout == 0 {
		joinTimeout = DefaultJoinTimeout
	}

	if connectTimeout < joinTimeout {
		// In case the connect timeout is smaller than the join timeout, align them
		connectTimeout = joinTimeout
	}

	// The join routine should be separate
	go func() {
		_ = source.Join(destination.AddrInfo(), joinTimeout)
	}()

	connectCtx, cancelFn := context.WithTimeout(context.Background(), connectTimeout)
	defer cancelFn()
	// Wait for the peer to be connected
	_, connectErr := WaitUntilPeerConnectsTo(connectCtx, source, destination.AddrInfo().ID)

	return connectErr
}

func WaitUntilPeerConnectsTo(ctx context.Context, srv *Server, ids ...peer.ID) (bool, error) {
	peersConnected := 0
	targetPeers := len(ids)

	res, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		for _, v := range ids {
			if srv.hasPeer(v) {
				peersConnected++
			}

			if peersConnected == targetPeers {
				return true, false
			}
		}
		return nil, true

	})
	if err != nil {
		return false, err
	}

	return res.(bool), nil
}

func WaitUntilPeerDisconnectsFrom(ctx context.Context, srv *Server, ids ...peer.ID) (bool, error) {
	peersDisconnected := 0
	targetPeers := len(ids)

	res, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		for _, v := range ids {
			if !srv.hasPeer(v) {
				peersDisconnected++
			}

			if peersDisconnected == targetPeers {
				return true, false
			}
		}
		return nil, true

	})
	if err != nil {
		return false, err
	}

	return res.(bool), nil
}

// constructMultiAddrs is a helper function for converting raw IPs to mutliaddrs
func constructMultiAddrs(addresses []string) ([]multiaddr.Multiaddr, error) {
	returnAddrs := make([]multiaddr.Multiaddr, 0)

	for _, addr := range addresses {
		multiAddr, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}

		returnAddrs = append(returnAddrs, multiAddr)
	}

	return returnAddrs, nil
}

func createServers(
	count int,
	paramsMap map[int]*CreateServerParams,
) ([]*Server, error) {
	servers := make([]*Server, count)
	if paramsMap == nil {
		paramsMap = map[int]*CreateServerParams{}
	}

	for i := 0; i < count; i++ {
		server, createErr := CreateServer(paramsMap[i])
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

// MeshJoin is a helper method for joining all the passed in servers into a mesh
func MeshJoin(servers ...*Server) []error {
	if len(servers) < 2 {
		return nil
	}

	// Join errors are used to gather all errors that happen
	// inside the go routines, so they can be handled when they finish
	joinErrors := make([]error, 0)
	var joinErrorsLock sync.Mutex

	appendJoinError := func(joinErr error) {
		joinErrorsLock.Lock()
		joinErrors = append(joinErrors, joinErr)
		joinErrorsLock.Unlock()
	}

	numServers := len(servers)
	var wg sync.WaitGroup
	for indx := 0; indx < numServers; indx++ {
		for innerIndx := 0; innerIndx < numServers; innerIndx++ {
			if innerIndx > indx {
				wg.Add(1)
				go func(src, dest int) {
					defer wg.Done()

					if joinErr := JoinAndWait(
						servers[src],
						servers[dest],
						DefaultBufferTimeout,
						DefaultJoinTimeout,
					); joinErr != nil {
						appendJoinError(fmt.Errorf("unable to join peers, %v", joinErr))
					}
				}(indx, innerIndx)
			}
		}
	}

	wg.Wait()
	return joinErrors
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

func closeTestServers(t *testing.T, servers []*Server) {
	for _, server := range servers {
		assert.NoError(t, server.Close())
	}
}
