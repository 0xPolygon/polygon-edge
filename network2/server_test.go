package network2

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/0xPolygon/minimal/network2/grpc/test"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/crypto"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
)

var startRangePort = uint64(10000)

func testServer(t *testing.T) *Server {
	port := atomic.AddUint64(&startRangePort, 1)

	priv, _, _ := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
	listen, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))

	config := &Config{
		PrivKey: priv,
		Addrs:   []ma.Multiaddr{listen},
	}
	logger := hclog.NewNullLogger()
	srv, err := NewServer(logger, config)
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func TestServer(t *testing.T) {
	srv0 := testServer(t)
	defer srv0.Close()

	srv1 := testServer(t)
	defer srv1.Close()

	srv0.AddPeerFromAddrInfo(srv1.AddrInfo())

	// connect srv0 to srv1
	m := &mockTest{}

	test.RegisterTestServer(srv0.grpcServer.GetGRPCServer(), m)
	test.RegisterTestServer(srv1.grpcServer.GetGRPCServer(), m)

	srv0.grpcServer.Serve()
	srv1.grpcServer.Serve()

	clt, err := srv0.Dial(srv1.AddrInfo().ID)
	assert.NoError(t, err)

	tClt := test.NewTestClient(clt)
	tClt.A(context.Background(), &test.AReq{})
}

type mockTest struct {
}

func (m *mockTest) A(ctx context.Context, req *test.AReq) (*test.AResp, error) {
	fmt.Println("-- a --")
	return &test.AResp{}, nil
}
