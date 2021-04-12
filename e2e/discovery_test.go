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

const NumOfNodes = 3

func TestDiscovery(t *testing.T) {
	srvs := make([]*framework.TestServer, NumOfNodes)
	for i := range srvs {
		srvs[i] = framework.NewTestServer(t, func(config *framework.TestServerConfig) {
		})
	}
	defer func() {
		for _, s := range srvs {
			s.Stop()
		}
	}()

	time.Sleep(5 * time.Second)

	p2pAddrs := make([]string, NumOfNodes)
	for i, s := range srvs {
		status, err := s.Operator().GetStatus(context.Background(), &empty.Empty{})
		if err != nil {
			t.Fatal(err)
		}
		p2pAddrs[i] = status.P2PAddr
	}

	for i := 0; i < NumOfNodes-1; i++ {
		srv, dest := srvs[i], p2pAddrs[i+1]
		_, err := srv.Operator().PeersAdd(context.Background(), &proto.PeersAddRequest{
			Id: dest,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(time.Second * 60)

	for i, s := range srvs {
		res, err := s.Operator().PeersList(context.Background(), &empty.Empty{})
		if err != nil {
			t.Fatal(err)
		}

		addrs := make([]string, len(res.Peers))
		for i, p := range res.Peers {
			addr, id := p.Addrs[0], p.Id
			addrs[i] = fmt.Sprintf("%s/p2p/%s", addr, id)
		}

		for j, target := range p2pAddrs {
			if i == j {
				continue
			}

			found := false
			for _, addr := range addrs {
				if addr == target {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Node %d couldn't find peer %s", i, target)
			}
		}
	}
}
