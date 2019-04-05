package ethereum

import (
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/blockchain"
)

/*
var status = &Status{
	ProtocolVersion: 63,
	NetworkID:       1,
	TD:              big.NewInt(1),
	CurrentBlock:    common.HexToHash("1"),
	GenesisBlock:    common.HexToHash("1"),
}

func testEthHandshake(t *testing.T, s0 *network.Server, b0 *blockchain.Blockchain, s1 *network.Server, b1 *blockchain.Blockchain) (*Ethereum, *Ethereum) {
	sts := func(b *blockchain.Blockchain) func() (*Status, error) {
		return func() (*Status, error) {
			s := status
			s.CurrentBlock = b.Header().Hash()
			return s, nil
		}
	}

	var eth0 *Ethereum
	c0 := func(conn net.Conn, p *network.Peer) protocol.Handler {
		eth0 = NewEthereumProtocol(conn, p, sts(b0), b0)
		return eth0
	}

	var eth1 *Ethereum
	c1 := func(conn net.Conn, p *network.Peer) protocol.Handler {
		eth1 = NewEthereumProtocol(conn, p, sts(b1), b1)
		return eth1
	}

	s0.RegisterProtocol(protocol.ETH63, c0)
	s1.RegisterProtocol(protocol.ETH63, c1)

	if err := s0.Schedule(); err != nil {
		t.Fatal(err)
	}
	if err := s1.Schedule(); err != nil {
		t.Fatal(err)
	}

	if err := s0.DialSync(s1.Enode.String()); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)
	return eth0, eth1
}
*/

func testPeerAncestor(t *testing.T, h0 []*types.Header, h1 []*types.Header, ancestor *types.Header) {
	b0 := blockchain.NewTestBlockchain(t, h0)
	b1 := blockchain.NewTestBlockchain(t, h1)

	syncer, err := NewBackend(nil, b0)
	if err != nil {
		t.Fatal(err)
	}

	eth0, _ := ethPipe(b0, b1)

	h, _, err := syncer.FindCommonAncestor(eth0)
	if err != nil {
		t.Fatal(err)
	}
	if ancestor == nil && h != nil {
		t.Fatal("expected nothing but header has content")
	}

	fmt.Println(h.Hash().String())
	fmt.Println(ancestor.Hash().String())

	if h.Hash() != ancestor.Hash() {
		t.Fatal("hash dont match")
	}
}

func TestPeerFindCommonAncestor(t *testing.T) {
	t.Run("Server with shorter chain", func(t *testing.T) {
		headers := blockchain.NewTestHeaderChain(1000)
		testPeerAncestor(t, headers[0:5], headers, headers[4])
	})

	t.Run("Server with shorter chain odd", func(t *testing.T) {
		headers := blockchain.NewTestHeaderChain(999)
		testPeerAncestor(t, headers[0:7], headers, headers[6])
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
		h1 := blockchain.NewTestHeaderChainWithSeed(nil, 100, 10)
		testPeerAncestor(t, h0, h1, nil)
	})

	t.Run("Ancestor is genesis", func(t *testing.T) {
		genesis := blockchain.NewTestHeaderChain(1)

		h0 := blockchain.NewTestHeaderFromChain(genesis, 10)
		h1 := blockchain.NewTestHeaderFromChainWithSeed(genesis, 10, 10)

		testPeerAncestor(t, h0, h1, genesis[0])
	})

	// TODO, ancestor with forked chain
}

/*
func newTestPeer(id string, pending int) *Peer {
	return &Peer{id: id, active: true, pending: pending, failed: 0}
}

func dequeuePeer(t *testing.T, s *Backend) *Peer {
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

func expectDequeue(t *testing.T, s *Backend, id string) {
	if found := dequeuePeer(t, s); found.id != id {
		t.Fatalf("expected to dequeue %s but found %s", id, found.id)
	}
}

func TestDequeueIncreasePending(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0 := blockchain.NewTestBlockchain(t, headers)

	s, err := NewBackend(1, b0, DefaultConfig())
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

	s, err := NewBackend(1, b0, DefaultConfig())
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

func TestDequeuePeerWithAwake(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0 := blockchain.NewTestBlockchain(t, headers)

	s, err := NewBackend(1, b0, DefaultConfig())
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
*/

func ethPipe(b0, b1 *blockchain.Blockchain) (*Ethereum, *Ethereum) {
	h0, _ := b0.Header()
	st0 := &Status{
		ProtocolVersion: 63,
		NetworkID:       1,
		TD:              big.NewInt(1),
		CurrentBlock:    h0.Hash(),
		GenesisBlock:    b0.Genesis().Hash(),
	}

	h1, _ := b1.Header()
	st1 := &Status{
		ProtocolVersion: 63,
		NetworkID:       1,
		TD:              big.NewInt(1),
		CurrentBlock:    h1.Hash(),
		GenesisBlock:    b1.Genesis().Hash(),
	}

	conn0, conn1 := net.Pipe()
	eth0 := NewEthereumProtocol(conn0, b0)
	eth1 := NewEthereumProtocol(conn1, b1)

	err := make(chan error)
	go func() {
		err <- eth0.Init(st0)
	}()
	go func() {
		err <- eth1.Init(st1)
	}()

	if err := <-err; err != nil {
		panic(err)
	}
	if err := <-err; err != nil {
		panic(err)
	}
	return eth0, eth1
}

func testEthereum(conn net.Conn, b *blockchain.Blockchain) *Ethereum {
	h, _ := b.Header()
	st := &status
	st.CurrentBlock = h.Hash()
	st.GenesisBlock = b.Genesis().Hash()

	eth := NewEthereumProtocol(conn, b)
	if err := eth.Init(st); err != nil {
		panic(err)
	}
	return eth
}

func TestBackendBroadcastBlock(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0 := blockchain.NewTestBlockchain(t, headers)

	b, err := NewBackend(nil, b0)
	if err != nil {
		panic(err)
	}

	c0, c1 := net.Pipe()

	var eth *Ethereum
	go func() {
		eth = testEthereum(c1, b0)
	}()
	if _, err := b.Add(c0, "1"); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	fmt.Println("-- eth --")
	fmt.Println(eth)

	/*
		watch := eth.Watch()

		req := b0.GetBlockByNumber(big.NewInt(100), true)
		b.broadcast(req)

		recv := <-watch
		if recv.Block.Number().Uint64() != req.Number().Uint64() {
			t.Fatal("bad")
		}
	*/
}

func TestBackendNotify(t *testing.T) {

	h0 := blockchain.NewTestHeaderChain(10)
	h1 := blockchain.NewTestHeaderFromChain(h0[1:5], 10)

	b1 := blockchain.HeadersToBlocks(h1)

	b := blockchain.NewTestBlockchain(t, h0)
	fmt.Println(b)

	if err := b.WriteBlocks(b1); err != nil {
		panic(err)
	}

	fmt.Println("-- forks --")
	fmt.Println(b.GetForks())
}

func TestBackendStuff(t *testing.T) {
	h0 := blockchain.NewTestHeaderChain(10)
	h1 := blockchain.NewTestHeaderFromChainWithSeed(h0[0:5], 10, 10)

	b0 := blockchain.NewTestBlockchain(t, h0)
	b1 := blockchain.NewTestBlockchain(t, h1)

	eth0, _ := ethPipe(b0, b1)

	b, err := NewBackend(nil, b0)
	if err != nil {
		t.Fatal(err)
	}

	p1 := &PeerConnection{
		conn:    eth0,
		sched:   b,
		id:      "1",
		enabled: false, // stop from running
	}
	b.peers["1"] = p1

	fmt.Println(b)

	target, _ := b1.GetBlockByNumber(big.NewInt(11), true)
	diff, _ := b1.GetTD(target.Hash())

	b.notifyNewData(&NotifyMsg{
		Block: target,
		Peer:  p1,
		Diff:  diff,
	})

	// The head has to be correct
	if b.queue.head.String() != h1[4].Hash().String() {
		t.Fatalf("bad")
	}

	// One of the forks is the fork between h0 and h1, since they dont have any
	// difficulty during tests, they are a match and nothign changes
	if b0.GetForks()[0].String() != h1[5].Hash().String() {
		t.Fatal("bad")
	}
}
