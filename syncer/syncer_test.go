package syncer

import (
	"math/big"
	"testing"
	"time"

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

func testEthHandshake(t *testing.T, s0 *network.Server, b0 *blockchain.Blockchain, s1 *network.Server, b1 *blockchain.Blockchain) (*ethereum.Ethereum, *ethereum.Ethereum) {
	sts := func(b *blockchain.Blockchain) func() (*ethereum.Status, error) {
		return func() (*ethereum.Status, error) {
			s := status
			s.CurrentBlock = b.Header().Hash()
			return s, nil
		}
	}

	var eth0 *ethereum.Ethereum
	c0 := func(s network.Conn, p *network.Peer) protocol.Handler {
		eth0 = ethereum.NewEthereumProtocol(s, p, sts(b0), b0)
		return eth0
	}

	var eth1 *ethereum.Ethereum
	c1 := func(s network.Conn, p *network.Peer) protocol.Handler {
		eth1 = ethereum.NewEthereumProtocol(s, p, sts(b1), b1)
		return eth1
	}

	s0.RegisterProtocol(protocol.ETH63, c0)
	s1.RegisterProtocol(protocol.ETH63, c1)

	s0.Dial(s1.Enode)

	time.Sleep(500 * time.Millisecond)
	return eth0, eth1
}

func testPeerAncestor(t *testing.T, h0 []*types.Header, h1 []*types.Header, ancestor *types.Header) {
	b0 := blockchain.NewTestBlockchain(t, h0)
	b1 := blockchain.NewTestBlockchain(t, h1)

	syncer, err := NewSyncer(1, b0, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}

	s0, s1 := network.TestServers()
	p0, _ := testEthHandshake(t, s0, b0, s1, b1)

	h, err := syncer.FindCommonAncestor(p0)
	if err != nil {
		t.Fatal(err)
	}
	if ancestor == nil && h != nil {
		t.Fatal("expected nothing but header has content")
	}
	if h.Hash() != ancestor.Hash() {
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
	b0 := blockchain.NewTestBlockchain(t, headers)

	// b1 with the whole chain
	b1 := blockchain.NewTestBlockchain(t, headers)

	syncer, err := NewSyncer(1, b0, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}

	s0, s1 := network.TestServers()
	p0, _ := testEthHandshake(t, s0, b0, s1, b1)

	height, err := syncer.fetchHeight(p0)
	if err != nil {
		t.Fatal(err)
	}
	if height.Number.Uint64() != 999 {
		t.Fatal("it should be 999")
	}
}

func newTestPeer(id string, pending int) *Peer {
	return &Peer{id: id, active: true, pending: pending, failed: 0}
}

func dequeuePeer(t *testing.T, s *Syncer) *Peer {
	// dequeue peer without blocking
	peer := make(chan *Peer, 1)
	go func() {
		p := s.dequeuePeer()
		peer <- p
	}()

	select {
	case p := <-peer:
		return p
	case <-time.After(100 * time.Millisecond):
		t.Fatal("it did not wake up")
	}

	return nil
}

func expectDequeue(t *testing.T, s *Syncer, id string) {
	if found := dequeuePeer(t, s); found.id != id {
		t.Fatalf("expected to dequeue %s but found %s", id, found.id)
	}
}

func TestDequeueIncreasePending(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0 := blockchain.NewTestBlockchain(t, headers)

	s, err := NewSyncer(1, b0, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	s.peers["a"] = newTestPeer("a", 0)

	expectDequeue(t, s, "a")
	if pending := s.peers["a"].pending; pending != 1 {
		t.Fatalf("pending should have been increased, expected 1 but found %d", pending)
	}
}

func TestDequeuePeers(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0 := blockchain.NewTestBlockchain(t, headers)

	s, err := NewSyncer(1, b0, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	s.peers["a"] = newTestPeer("a", 0)
	s.peers["b"] = newTestPeer("b", 3)

	expectDequeue(t, s, "a")
	expectDequeue(t, s, "a")
	expectDequeue(t, s, "a")
	expectDequeue(t, s, "a")
	expectDequeue(t, s, "b")
}

func TestDequeuPeerWithAwake(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0 := blockchain.NewTestBlockchain(t, headers)

	s, err := NewSyncer(1, b0, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	s.peers["a"] = newTestPeer("a", 5)

	peer := make(chan *Peer, 1)
	go func() {
		p := s.dequeuePeer()
		peer <- p
	}()

	// awake peer a
	s.ack(s.peers["a"], false)

	select {
	case p := <-peer:
		if p.id != "a" {
			t.Fatalf("wrong peer woke up, expected a but found %s", p.id)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("it did not wake up")
	}
}
