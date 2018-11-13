package ethereum

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/protocol"
)

func testEthHandshake(t *testing.T, s0 *network.Server, ss0 *Status, b0 *blockchain.Blockchain, s1 *network.Server, ss1 *Status, b1 *blockchain.Blockchain) (*Ethereum, *Ethereum) {
	st0 := func() (*Status, error) {
		return ss0, nil
	}

	st1 := func() (*Status, error) {
		return ss1, nil
	}

	var eth0 *Ethereum
	c0 := func(s network.Conn, p *network.Peer) protocol.Handler {
		eth0 = NewEthereumProtocol(s, p, st0, b0)
		return eth0
	}

	var eth1 *Ethereum
	c1 := func(s network.Conn, p *network.Peer) protocol.Handler {
		eth1 = NewEthereumProtocol(s, p, st1, b1)
		return eth1
	}

	s0.RegisterProtocol(protocol.ETH63, c0)
	s1.RegisterProtocol(protocol.ETH63, c1)

	s0.Dial(s1.Enode)

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
		eth0, eth1 := testEthHandshake(t, s0, cc.Status0, nil, s1, cc.Status1, nil)

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
	}
}

func TestHandshakeMsgPostHandshake(t *testing.T) {
	// After the handshake we dont accept more handshake messages
	s0, s1 := network.TestServers()
	eth0, _ := testEthHandshake(t, s0, &status, nil, s1, &status, nil)

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

func headersToNumbers(headers []*types.Header) []int {
	n := []int{}
	for _, h := range headers {
		n = append(n, int(h.Number.Int64()))
	}
	return n
}

func TestEthereumBlockHeadersMsg(t *testing.T) {
	b0, close0 := blockchain.NewTestBlockchain(t)
	defer close0()

	b1, close1 := blockchain.NewTestBlockchain(t)
	defer close1()

	s0, s1 := network.TestServers()
	eth0, _ := testEthHandshake(t, s0, &status, b0, s1, &status, b1)

	genesis := &types.Header{Number: big.NewInt(0)}
	headers := []*types.Header{genesis}

	// populate b1 with headers
	for i := 1; i < 100; i++ {
		headers = append(headers, &types.Header{ParentHash: headers[i-1].Hash(), Number: big.NewInt(int64(i)), Difficulty: big.NewInt(int64(i))})
	}

	if err := b1.WriteGenesis(genesis); err != nil {
		t.Fatal(err)
	}
	if err := b1.WriteHeaders(headers[1:]); err != nil {
		t.Fatal(err)
	}

	var cases = []struct {
		origin   interface{}
		amount   uint64
		skip     uint64
		reverse  bool
		expected []int
	}{
		{
			headers[1].Hash(),
			10,
			5,
			false,
			[]int{1, 6, 11, 16, 21, 26, 31, 36, 41, 46},
		},
		{
			1,
			10,
			5,
			false,
			[]int{1, 6, 11, 16, 21, 26, 31, 36, 41, 46},
		},
	}

	for _, cc := range cases {
		t.Run("", func(tt *testing.T) {
			ack := make(chan network.AckMessage, 1)
			eth0.Conn().SetHandler(BlockHeadersMsg, ack, 5*time.Second)

			var err error
			if reflect.TypeOf(cc.origin).Name() == "Hash" {
				err = eth0.RequestHeadersByHash(cc.origin.(common.Hash), cc.amount, cc.skip, cc.reverse)
			} else {
				err = eth0.RequestHeadersByNumber(uint64(cc.origin.(int)), cc.amount, cc.skip, cc.reverse)
			}

			if err != nil {
				t.Fatal(err)
			}

			resp := <-ack
			if resp.Complete {
				var result []*types.Header
				if err := rlp.DecodeBytes(resp.Payload, &result); err != nil {
					t.Fatal(err)
				}

				if !reflect.DeepEqual(headersToNumbers(result), cc.expected) {
					t.Fatal("expected numbers dont match")
				}
			} else {
				t.Fatal("failed to receive the headers")
			}
		})
	}
}
