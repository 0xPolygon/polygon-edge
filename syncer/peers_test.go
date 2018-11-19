package syncer

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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
	sts := func(b *blockchain.Blockchain) func() (*ethereum.Status, error) {
		return func() (*ethereum.Status, error) {
			s := status
			s.CurrentBlock = b.Header().Hash()
			return s, nil
		}
	}

	var peer0 *network.Peer
	c0 := func(s network.Conn, p *network.Peer) protocol.Handler {
		peer0 = p
		return ethereum.NewEthereumProtocol(s, p, sts(b0), b0)
	}

	var peer1 *network.Peer
	c1 := func(s network.Conn, p *network.Peer) protocol.Handler {
		peer1 = p
		return ethereum.NewEthereumProtocol(s, p, sts(b1), b1)
	}

	s0.RegisterProtocol(protocol.ETH63, c0)
	s1.RegisterProtocol(protocol.ETH63, c1)

	if err := s0.DialSync(s1.Enode); err != nil {
		t.Fatal(err)
	}

	// p0 is the connection reference for server 0 to peer 1
	p0 := NewPeer(peer0, nil)
	// p1 is the connection reference for server 1 to peer 0
	p1 := NewPeer(peer1, nil)

	return p0, p1
}

func TestPeerConcurrentHeaderCalls(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)

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
			h, err := p0.requestHeaders(i, 100)
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
	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0, close0 := blockchain.NewTestBlockchain(t, headers[0:5])
	defer close0()

	// b1 with the whole chain
	b1, close1 := blockchain.NewTestBlockchain(t, headers)
	defer close1()

	s0, s1 := network.TestServers()
	p0, _ := testPeers(t, s0, b0, s1, b1)

	if _, err := p0.requestHeaders(1100, 100); err == nil {
		t.Fatal("it should fail")
	}

	// NOTE: We cannot know from an empty response which is the
	// pending block it belongs to (because we use the first block to know the origin)
	// Thus, the query will fail with a timeout message but it does not
	// mean the peer is timing out on the responses.
}

func TestPeerCloseConnection(t *testing.T) {
	// close the connection while doing the request

	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0, close0 := blockchain.NewTestBlockchain(t, headers[0:5])
	defer close0()

	// b1 with the whole chain
	b1, close1 := blockchain.NewTestBlockchain(t, headers)
	defer close1()

	s0, s1 := network.TestServers()
	p0, _ := testPeers(t, s0, b0, s1, b1)

	if _, err := p0.requestHeaders(0, 100); err != nil {
		t.Fatal(err)
	}

	s1.Close()
	if _, err := p0.requestHeaders(100, 100); err == nil {
		t.Fatal("it should fail after the connection has been closed")
	}
}

func testPeerAncestor(t *testing.T, h0 []*types.Header, h1 []*types.Header, header *types.Header) {
	// b0 with only the genesis
	b0, close0 := blockchain.NewTestBlockchain(t, h0)
	defer close0()

	// b1 with the whole chain
	b1, close1 := blockchain.NewTestBlockchain(t, h1)
	defer close1()

	syncer, err := NewSyncer(1, b0, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}

	s0, s1 := network.TestServers()
	p0, _ := testPeers(t, s0, b0, s1, b1)
	p0.syncer = syncer

	h, err := p0.FindCommonAncestor()
	if err != nil {
		t.Fatal(err)
	}
	if header == nil && h != nil {
		fmt.Println(h)
		t.Fatal("expected nothing but header has content")
	}
	if h.Hash() != header.Hash() {
		t.Fatal("hash dont match")
	}
}

func TestPeerFindCommonAncestor(t *testing.T) {
	t.Run("Server with shorter chain", func(t *testing.T) {
		headers := blockchain.NewTestHeaderChain(1000)
		testPeerAncestor(t, headers[0:5], headers, headers[4])
	})

	t.Run("Server with longer chain", func(t *testing.T) {
		headers := blockchain.NewTestHeaderChain(1000)
		testPeerAncestor(t, headers, headers[0:5], headers[4])
	})

	t.Run("Same chain", func(t *testing.T) {
		headers := blockchain.NewTestHeaderChain(100)
		testPeerAncestor(t, headers, headers, headers[len(headers)-1])
	})

	t.Run("No matches", func(t *testing.T) {
		h0 := blockchain.NewTestHeaderChain(100)
		h1 := blockchain.NewTestHeaderChainWithSeed(100, 10)
		testPeerAncestor(t, h0, h1, nil)
	})

	// TODO, ancestor with forked chain
}

func TestPeerHeight(t *testing.T) {

	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0, close0 := blockchain.NewTestBlockchain(t, headers)
	defer close0()

	// b1 with the whole chain
	b1, close1 := blockchain.NewTestBlockchain(t, headers)
	defer close1()

	s0, s1 := network.TestServers()
	p0, _ := testPeers(t, s0, b0, s1, b1)

	height, err := p0.fetchHeight()
	if err != nil {
		t.Fatal(err)
	}
	if height.Number.Uint64() != 999 {
		t.Fatal("it should be 999")
	}
}

func TestPeerReceiptsAndBodies(t *testing.T) {
	headers, bodies, receipts := blockchain.NewTestBodyChain(1000)

	// b0 with only the genesis
	b0, close0 := blockchain.NewTestBlockchain(t, blockchain.NewTestHeaderChain(1000))
	defer close0()

	// b1 with the whole chain
	b1, close1 := blockchain.NewTestBlockchainWithBlocks(t, bodies, receipts)
	defer close1()

	s0, s1 := network.TestServers()
	p0, _ := testPeers(t, s0, b0, s1, b1)

	// start from 1 to avoid genesis block
	bb, err := p0.requestReceipts(headers[1:11])
	if err != nil {
		t.Fatal(err)
	}
	if len(bb) != 10 {
		t.Fatal("expected 10 items")
	}

	r0, err := p0.requestBodies(headers[1:11])
	if err != nil {
		t.Fatal(err)
	}
	if len(r0) != 10 {
		t.Fatal("expected 10 items")
	}
}
