package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

func TestDiscovery(t *testing.T) {
	tests := []struct {
		name     string
		numNodes int
		// Number of nodes that connects to left node as default
		numInitConnectNodes int
	}{
		{
			name:                "first 4 nodes should know each other",
			numNodes:            5,
			numInitConnectNodes: 4,
		},
		{
			name:                "all should know each other",
			numNodes:            5,
			numInitConnectNodes: 5,
		},
	}

	conf := func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDummy)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvs := make([]*framework.TestServer, tt.numNodes)
			for i := range srvs {
				srvs[i] = framework.NewTestServer(t, conf)
			}
			defer func() {
				for _, s := range srvs {
					s.Stop()
				}
			}()

			p2pAddrs := make([]string, tt.numNodes)
			for i, s := range srvs {
				status, err := s.Operator().GetStatus(context.Background(), &empty.Empty{})
				if err != nil {
					t.Fatal(err)
				}
				p2pAddrs[i] = status.P2PAddr
			}

			for i := 0; i < tt.numInitConnectNodes-1; i++ {
				srv, dest := srvs[i], p2pAddrs[i+1]
				_, err := srv.Operator().PeersAdd(context.Background(), &proto.PeersAddRequest{
					Id: dest,
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			time.Sleep(10 * time.Second)

			for i, s := range srvs {
				res, err := s.Operator().PeersList(context.Background(), &empty.Empty{})
				if err != nil {
					t.Fatal(err)
				}

				isAddrKnown := make(map[string]bool, len(res.Peers))
				for _, p := range res.Peers {
					addr, id := p.Addrs[0], p.Id
					key := fmt.Sprintf("%s/p2p/%s", addr, id)
					isAddrKnown[key] = true
				}

				for j, addr := range p2pAddrs {
					shouldKnow := i != j && i < tt.numInitConnectNodes && j < tt.numInitConnectNodes
					actual := isAddrKnown[addr]

					if shouldKnow != actual {
						if shouldKnow {
							t.Errorf("node %d should know node %d, but doesn't know", i, j)
						} else {
							t.Errorf("node %d shouldn't know node %d, but knows", i, j)
						}
					}
				}
			}
		})
	}
}
