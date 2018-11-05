package ethereum

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/protocol"
)

func testEthHandshake(t *testing.T, s0 *network.Server, ss0 *Status, s1 *network.Server, ss1 *Status) (*Ethereum, *Ethereum) {
	st0 := func() (*Status, error) {
		return ss0, nil
	}

	st1 := func() (*Status, error) {
		return ss1, nil
	}

	var eth0 *Ethereum
	c0 := func(s network.Conn, p *network.Peer) protocol.Handler {
		eth0 = NewEthereumProtocol(s, p, st0)
		return eth0
	}

	var eth1 *Ethereum
	c1 := func(s network.Conn, p *network.Peer) protocol.Handler {
		eth1 = NewEthereumProtocol(s, p, st1)
		return eth1
	}

	s0.RegisterProtocol(protocol.ETH63, c0)
	s1.RegisterProtocol(protocol.ETH63, c1)

	if err := s0.Dial(s1.Enode); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)
	return eth0, eth1
}

var status = Status{
	ProtocolVersion: 63,
	NetworkID:       1,
	TD:              big.NewInt(1),
	CurrentBlock:    common.HexToHash("1"),
	GenesisBlock:    common.HexToHash("1"),
}

func TestHandshake(t *testing.T) {

	// Networkid is different
	status1 := status
	status1.NetworkID = 2

	// Current block is different
	status2 := status
	status2.CurrentBlock = common.HexToHash("2")

	// Genesis block is different
	status3 := status
	status3.GenesisBlock = common.HexToHash("2")

	cases := []struct {
		Status0  *Status
		Status1  *Status
		Expected bool
	}{
		{
			&status,
			&status,
			true,
		},
		{
			&status,
			&status1,
			false,
		},
		{
			&status,
			&status2,
			true,
		},
		{
			&status,
			&status3,
			false,
		},
	}

	for _, cc := range cases {
		s0, s1 := network.TestServers()
		eth0, eth1 := testEthHandshake(t, s0, cc.Status0, s1, cc.Status1)

		// Both handshake fail
		evnt0, evnt1 := <-s0.EventCh, <-s1.EventCh
		if cc.Expected && evnt0.Type != network.NodeJoin {
			t.Fatal("expected to work but not")
		}
		if cc.Expected && evnt1.Type != network.NodeJoin {
			t.Fatal("expected to work but not")
		}

		// If it worked, check if the status message we get is the good one
		if cc.Expected {
			if !reflect.DeepEqual(eth0.status, cc.Status1) {
				t.Fatal("bad")
			}
			if !reflect.DeepEqual(eth1.status, cc.Status0) {
				t.Fatal("bad")
			}
		}

		// If it did not work, check that the peers are disconnected
		if !cc.Expected {
			// Check if they are still connected
			p0 := s0.GetPeer(s1.ID().String())
			if p0.Connected == true {
				t.Fatal("should be disconnected")
			}

			p1 := s1.GetPeer(s0.ID().String())
			if p1.Connected == true {
				t.Fatal("should be disconnected")
			}
		}
	}
}

func TestHandshakeMsgPostHandshake(t *testing.T) {
	// After the handshake we dont accept more handshake messages
	s0, s1 := network.TestServers()
	eth0, _ := testEthHandshake(t, s0, &status, s1, &status)

	if err := eth0.conn.WriteMsg(StatusMsg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	// Check if they are still connected
	p0 := s0.GetPeer(s1.ID().String())
	if p0.Connected == true {
		t.Fatal("should be disconnected")
	}

	p1 := s1.GetPeer(s0.ID().String())
	if p1.Connected == true {
		t.Fatal("should be disconnected")
	}
}

// TODO Test: Message send.
// TODO Test: Ethereum stops handshake if parity protocol is also enabled.
