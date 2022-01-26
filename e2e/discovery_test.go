package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/server/proto"
	empty "google.golang.org/protobuf/types/known/emptypb"
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
			srvs := framework.NewTestServers(t, tt.numNodes, conf)

			p2pAddrs := make([]string, tt.numNodes)
			for i, s := range srvs {
				status, err := s.Operator().GetStatus(context.Background(), &empty.Empty{})
				if err != nil {
					t.Fatal(err)
				}
				p2pAddrs[i] = strings.Split(status.P2PAddr, ",")[0]
			}

			for i := 0; i < tt.numInitConnectNodes-1; i++ {
				srv, dest := srvs[i], p2pAddrs[i+1]
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := srv.Operator().PeersAdd(ctx, &proto.PeersAddRequest{
					Id: dest,
				})
				if err != nil {
					t.Fatal(err)
				}
			}

			for i, srv := range srvs {
				shouldKnowPeers := true
				subTestName := fmt.Sprintf("node %d should know other peers", i)
				if i >= tt.numInitConnectNodes {
					shouldKnowPeers = false
					subTestName = fmt.Sprintf("node %d shouldn't know other peers", i)
				}

				t.Run(subTestName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					res, err := framework.WaitUntilPeerConnects(ctx, srv, tt.numInitConnectNodes-1)

					if err != nil {
						if shouldKnowPeers {
							t.Error(err)
						} else {
							// server expected to be isolated
							return
						}
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
				})
			}
		})
	}
}
