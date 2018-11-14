package syncer

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/protocol"
	"github.com/umbracle/minimal/protocol/ethereum"
)

var status = &ethereum.Status{
	ProtocolVersion: 63,
	NetworkID:       1,
	TD:              big.NewInt(1),
	CurrentBlock:    common.HexToHash("1"),
	GenesisBlock:    common.HexToHash("1"),
}

func testPeers(t *testing.T, s0 *network.Server, b0 *blockchain.Blockchain, s1 *network.Server, b1 *blockchain.Blockchain) (*Peer, *Peer) {
	st := func() (*ethereum.Status, error) {
		return status, nil
	}

	var peer0 *network.Peer
	c0 := func(s network.Conn, p *network.Peer) protocol.Handler {
		peer0 = p
		return ethereum.NewEthereumProtocol(s, p, st, b0)
	}

	var peer1 *network.Peer
	c1 := func(s network.Conn, p *network.Peer) protocol.Handler {
		peer1 = p
		return ethereum.NewEthereumProtocol(s, p, st, b1)
	}

	s0.RegisterProtocol(protocol.ETH63, c0)
	s1.RegisterProtocol(protocol.ETH63, c1)

	s0.Dial(s1.Enode)

	time.Sleep(500 * time.Millisecond)

	p0 := NewPeer(peer0, nil, nil)
	p1 := NewPeer(peer1, nil, nil)

	return p0, p1
}

func TestPeerConcurrentHeaderCalls(t *testing.T) {
	headers := blockchain.NewTestChain(1000)

	// b0 with only the genesis
	b0, close0 := blockchain.NewTestBlockchain(t, headers[0:5])
	defer close0()

	// b1 with the whole chain
	b1, close1 := blockchain.NewTestBlockchain(t, headers)
	defer close1()

	s0, s1 := network.TestServers()
	p0, _ := testPeers(t, s0, b0, s1, b1)

	cases := []uint64{10}
	errr := make(chan error, len(cases))

	for indx, i := range cases {
		go func(indx int, i uint64) {
			h, err := p0.requestJob(Job{uint32(indx), i})
			if err == nil {
				if len(h) != 100 {
					err = fmt.Errorf("length not correct")
				} else {
					for indx, j := range h {
						if j.Number.Uint64() != i+uint64(indx) {
							err = fmt.Errorf("numbers dont match")
							break
						}
					}
				}
			}
			errr <- err
		}(indx, i)
	}

	for i := 0; i < len(cases); i++ {
		if err := <-errr; err != nil {
			t.Fatal(err)
		}
	}
}

func TestPeerEmptyResponseFails(t *testing.T) {
	headers := blockchain.NewTestChain(1000)

	// b0 with only the genesis
	b0, close0 := blockchain.NewTestBlockchain(t, headers[0:5])
	defer close0()

	// b1 with the whole chain
	b1, close1 := blockchain.NewTestBlockchain(t, headers)
	defer close1()

	s0, s1 := network.TestServers()
	p0, _ := testPeers(t, s0, b0, s1, b1)

	if _, err := p0.requestJob(Job{0, 1100}); err == nil {
		t.Fatal("it should fail")
	}

	// NOTE: We cannot know from an empty response which is the
	// pending block it belongs to (because we use the first block to know the origin)
	// Thus, the query will fail with a timeout message but it does not
	// mean the peer is timing out on the responses.
}

func TestPeerCloseConnection(t *testing.T) {
	// close the connection while doing the request

	headers := blockchain.NewTestChain(1000)

	// b0 with only the genesis
	b0, close0 := blockchain.NewTestBlockchain(t, headers[0:5])
	defer close0()

	// b1 with the whole chain
	b1, close1 := blockchain.NewTestBlockchain(t, headers)
	defer close1()

	s0, s1 := network.TestServers()
	p0, _ := testPeers(t, s0, b0, s1, b1)

	if _, err := p0.requestJob(Job{1, 0}); err != nil {
		t.Fatal(err)
	}

	s1.Close()
	if _, err := p0.requestJob(Job{2, 100}); err == nil {
		t.Fatal("it should fail after the connection has been closed")
	}
}
