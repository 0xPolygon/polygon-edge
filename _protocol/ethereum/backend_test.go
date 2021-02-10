package ethereum

import (
	"io/ioutil"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func newTestBackend(t *testing.T, b0 *blockchain.Blockchain) *Backend {
	logger := hclog.New(&hclog.LoggerOptions{
		Output: ioutil.Discard,
	})

	syncer, err := NewBackend(nil, logger, b0)
	assert.NoError(t, err)

	return syncer
}

func testPeerAncestor(t *testing.T, h0 []*types.Header, h1 []*types.Header, ancestor *types.Header) {
	b0 := blockchain.NewTestBlockchain(t, h0)
	b1 := blockchain.NewTestBlockchain(t, h1)

	syncer := newTestBackend(t, b0)

	eth0, _ := ethPipe(b0, b1)

	height, err := eth0.fetchHeight2()
	if err != nil {
		t.Fatal(err)
	}
	h, err := syncer.FindCommonAncestor(eth0, height)
	if err != nil {
		t.Fatal(err)
	}
	if ancestor == nil && h != nil {
		t.Fatal("expected nothing but header has content")
	}

	if h.Hash != ancestor.Hash {
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

	t.Run("Ancestor is genesis", func(t *testing.T) {
		genesis := blockchain.NewTestHeaderChain(1)

		h0 := blockchain.NewTestHeaderFromChain(genesis, 10)
		h1 := blockchain.NewTestHeaderFromChainWithSeed(genesis, 10, 10)

		testPeerAncestor(t, h0, h1, genesis[0])
	})

	t.Run("Empty chain", func(t *testing.T) {
		genesis := blockchain.NewTestHeaderChain(1)

		h0 := blockchain.NewTestHeaderFromChain(genesis, 10)
		h1 := blockchain.NewTestHeaderFromChain(genesis, 0)

		testPeerAncestor(t, h0, h1, genesis[0])
	})
	// TODO, ancestor with forked chain
}

func TestMaxConcurrentTasks(t *testing.T) {
	b0 := blockchain.NewTestBlockchain(t, blockchain.NewTestHeaderChain(1000))

	b := newTestBackend(t, b0)

	peekCh := func(b *Backend) bool {
		workerCh := make(chan *worker)
		go func() {
			w := b.peek()
			workerCh <- w
		}()

		select {
		case <-workerCh:
			return true
		case <-time.After(10 * time.Millisecond):
			return false
		}
	}

	// Add enough peers to reach max concurrent tasks
	for _, p := range []string{"1", "2", "3", "4"} {
		b.addPeer(p, nil)
	}
	for i := 0; i < maxConcurrentTasks; i++ {
		b.peek()
	}

	// No peek if maxConcurrentTasks reached
	assert.False(t, peekCh(b))

	b = newTestBackend(t, b0)

	// Add enough peers to reach max concurrent tasks
	for _, p := range []string{"1", "2"} {
		b.addPeer(p, nil)
	}
	for i := 0; i < maxOutstandingRequests*2; i++ {
		b.peek()
	}

	// No peek if all the peers are busy
	assert.False(t, peekCh(b))
}

func TestPeerDequeueIncreaseOutstandingCount(t *testing.T) {
	// Every new peek should increase the outstanding request count

	b0 := blockchain.NewTestBlockchain(t, blockchain.NewTestHeaderChain(1000))
	b := newTestBackend(t, b0)

	peers := map[string]int{}
	for _, p := range []string{"1", "2"} {
		b.addPeer(p, nil)
		peers[p] = 1
	}

	for i := 0; i < maxOutstandingRequests*2; i++ {
		w := b.peek()
		assert.Equal(t, peers[w.id], w.outstanding)
		peers[w.id]++
	}
}

func ethPipe(b0, b1 *blockchain.Blockchain) (*Ethereum, *Ethereum) {
	h0, _ := b0.Header()
	st0 := &Status{
		ProtocolVersion: 63,
		NetworkID:       1,
		TD:              big.NewInt(1),
		CurrentBlock:    h0.Hash,
		GenesisBlock:    b0.Genesis(),
	}

	h1, _ := b1.Header()
	st1 := &Status{
		ProtocolVersion: 63,
		NetworkID:       1,
		TD:              big.NewInt(1),
		CurrentBlock:    h1.Hash,
		GenesisBlock:    b1.Genesis(),
	}

	conn0, conn1 := net.Pipe()
	eth0 := newTestEthereumProto("", conn0, b0)
	eth1 := newTestEthereumProto("", conn1, b1)

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
	st.CurrentBlock = h.Hash
	st.GenesisBlock = b.Genesis()

	eth := newTestEthereumProto("", conn, b)
	if err := eth.Init(st); err != nil {
		panic(err)
	}
	return eth
}

func TestWorkerPool(t *testing.T) {
	pool := newWorkersHeap()

	// pool is nil the first time
	assert.Nil(t, pool.Peek())

	// push and peek
	assert.NoError(t, pool.Push("1", nil))
	assert.Equal(t, pool.Peek().id, "1")

	assert.NoError(t, pool.Push("2", nil))
	assert.Len(t, pool.index, 2)

	// Remove '1'. Only one element left
	pool.Remove("1")
	assert.Len(t, pool.index, 1)
	assert.Equal(t, pool.Peek().id, "2")
}
