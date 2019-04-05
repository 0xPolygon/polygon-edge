package ethereum

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"reflect"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/blockchain"
)

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
		Name    string
		Status0 *Status
		Status1 *Status
		Error   bool
	}{
		{
			Name:    "Same status",
			Status0: &status,
			Status1: &status,
			Error:   false,
		},
		{
			Name:    "Different network id",
			Status0: &status,
			Status1: &status1,
			Error:   true,
		},
		{
			Name:    "Different current block",
			Status0: &status,
			Status1: &status2,
			Error:   false,
		},
		{
			Name:    "Different genesis block",
			Status0: &status,
			Status1: &status3,
			Error:   true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			eth0, err0, eth1, err1 := testEthHandshakeWithStatus(c.Status0, nil, c.Status1, nil)

			if err0 != err1 && (err0 == nil || err1 == nil) {
				t.Fatal("errors dont match")
			}
			if err0 != nil && !c.Error {
				t.Fatalf("error not expected: %v", err0)
			} else if err0 == nil && c.Error {
				t.Fatal("expected error")
			}

			if !c.Error {
				if !reflect.DeepEqual(eth0.status, c.Status1) {
					t.Fatal("bad")
				}
				if !reflect.DeepEqual(eth1.status, c.Status0) {
					t.Fatal("bad")
				}
			}
		})
	}
}

func testEthHandshakeWithStatus(ss0 *Status, b0 *blockchain.Blockchain, ss1 *Status, b1 *blockchain.Blockchain) (*Ethereum, error, *Ethereum, error) {
	conn0, conn1 := net.Pipe()

	eth0 := NewEthereumProtocol(conn0, b0)
	eth1 := NewEthereumProtocol(conn1, b1)

	err := make(chan error)
	go func() {
		err <- eth0.Init(ss0)
	}()
	go func() {
		err <- eth1.Init(ss1)
	}()

	err0 := <-err
	err1 := <-err

	return eth0, err0, eth1, err1
}

func testEthProtocol(t *testing.T, b0 *blockchain.Blockchain, b1 *blockchain.Blockchain) (*Ethereum, *Ethereum) {
	eth0, err0, eth1, err1 := testEthHandshakeWithStatus(&status, b0, &status, b1)
	if err0 != nil || err1 != nil {
		t.Fatal("bad")
	}
	return eth0, eth1
}

func headersToNumbers(headers []*types.Header) []int {
	n := []int{}
	for _, h := range headers {
		n = append(n, int(h.Number.Int64()))
	}
	return n
}

func TestEthereumBlockHeadersMsg(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(100)

	b0 := blockchain.NewTestBlockchain(t, headers)
	b1 := blockchain.NewTestBlockchain(t, headers)

	eth0, _ := testEthProtocol(t, b0, b1)

	var cases = []struct {
		Origin   interface{}
		Amount   uint64
		Skip     uint64
		Reverse  bool
		Expected []int
	}{
		/*
			{
				Origin:   headers[1].Hash(),
				Amount:   10,
				Skip:     4,
				Reverse:  false,
				Expected: []int{1, 6, 11, 16, 21, 26, 31, 36, 41, 46},
			},
		*/
		{
			Origin:   1,
			Amount:   10,
			Skip:     4,
			Reverse:  false,
			Expected: []int{1, 6, 11, 16, 21, 26, 31, 36, 41, 46},
		},
		{
			Origin:   1,
			Amount:   10,
			Skip:     0,
			Reverse:  false,
			Expected: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			ack := make(chan AckMessage, 1)

			var err error
			if reflect.TypeOf(c.Origin).Name() == "Hash" {
				hash := c.Origin.(common.Hash)
				eth0.setHandler(context.Background(), headerMsg, hash.String(), ack)
				err = eth0.RequestHeadersByHash(hash, c.Amount, c.Skip, c.Reverse)
			} else {
				number := uint64(c.Origin.(int))
				eth0.setHandler(context.Background(), headerMsg, strconv.Itoa(int(number)), ack)
				err = eth0.RequestHeadersByNumber(number, c.Amount, c.Skip, c.Reverse)
			}

			if err != nil {
				t.Fatal(err)
			}

			resp := <-ack
			if resp.Complete {
				if !reflect.DeepEqual(headersToNumbers(resp.Result.([]*types.Header)), c.Expected) {
					t.Fatal("expected numbers dont match")
				}
			} else {
				t.Fatal("failed to receive the headers")
			}
		})
	}
}

func TestEthereumEmptyResponseBodyAndReceipts(t *testing.T) {
	// There are no body and no receipts, the answer is empty
	headers := blockchain.NewTestHeaderChain(100)

	b0 := blockchain.NewTestBlockchain(t, headers)
	b1 := blockchain.NewTestBlockchain(t, headers)

	eth0, _ := testEthProtocol(t, b0, b1)

	batch := []common.Hash{
		headers[0].Hash(),
		headers[5].Hash(),
		headers[10].Hash(),
	}

	// bodies
	ack := make(chan AckMessage, 1)
	eth0.setHandler(context.Background(), bodyMsg, batch[0].String(), ack)

	if err := eth0.RequestBodies(batch); err != nil {
		t.Fatal(err)
	}
	// NOTE, we cannot know if something failed yet, so empty response
	// means checking if the timeout fails
	if resp := <-ack; resp.Complete {
		t.Fatal("bad")
	}

	// receipts
	ack = make(chan AckMessage, 1)
	eth0.setHandler(context.Background(), receiptsMsg, batch[0].String(), ack)

	if err := eth0.RequestReceipts(batch); err != nil {
		t.Fatal(err)
	}
	if resp := <-ack; resp.Complete {
		t.Fatal("bad")
	}
}

func TestEthereumBody(t *testing.T) {
	b0 := blockchain.NewTestBlockchain(t, blockchain.NewTestHeaderChain(100))

	headers, blocks, receipts := blockchain.NewTestBodyChain(3) // only s1 needs to have bodies and receipts
	b1 := blockchain.NewTestBlockchainWithBlocks(t, blocks, receipts)

	eth0, _ := testEthProtocol(t, b0, b1)

	// NOTE, we use tx to check if the response is correct, genesis does not
	// have any tx so if we use that one it will fail
	batch := []uint64{2}

	msg := []common.Hash{}
	for _, i := range batch {
		msg = append(msg, headers[i].Hash())
	}

	first := blocks[batch[0]]
	hash := encodeHash(types.CalcUncleHash(first.Uncles()), types.DeriveSha(types.Transactions(first.Transactions()))).String()

	// -- bodies --

	ack := make(chan AckMessage, 1)
	eth0.setHandler(context.Background(), bodyMsg, hash, ack)

	if err := eth0.RequestBodies(msg); err != nil {
		t.Fatal(err)
	}

	resp := <-ack
	if !resp.Complete {
		t.Fatal("not completed")
	}
	bodies := resp.Result.(BlockBodiesData)
	if len(bodies) != len(batch) {
		t.Fatal("bodies: length is not correct")
	}
	for indx := range batch {
		if batch[indx] != bodies[indx].Transactions[0].Nonce() {
			t.Fatal("numbers dont match")
		}
	}

	// -- receipts --
	/*
		hash = types.DeriveSha(types.Receipts(receipts[0])).String()

		ack = make(chan AckMessage, 1)
		eth0.setHandler(hash, 1, ack)

		if err := eth0.RequestReceipts(msg); err != nil {
			t.Fatal(err)
		}

		resp = <-ack
		if !resp.Complete {
			t.Fatal("not completed")
		}
		receiptsResp := resp.Result.([][]*types.Receipt)
		if len(receiptsResp) != len(batch) {
			t.Fatal("receipts: length is not correct")
		}
		for indx, i := range batch {
			// cumulativegasused is the index of the block to which the receipt belongs
			if i != receiptsResp[indx][0].CumulativeGasUsed {
				t.Fatal("error")
			}
		}
	*/
}

/*
func TestHandshakeMsgPostHandshake(t *testing.T) {
	// After the handshake we dont accept more handshake messages
	s0, s1 := network.TestServers(network.DefaultConfig())
	eth0, _ := testEthHandshake(t, s0, &status, nil, s1, &status, nil)

	if err := eth0.conn.WriteMsg(StatusMsg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	// Check if they are still connected
	p0 := s0.GetPeer(s1.ID().String())
	if !p0.IsClosed() {
		t.Fatal("p0 should be disconnected")
	}

	p1 := s1.GetPeer(s0.ID().String())
	if !p1.IsClosed() {
		t.Fatal("p1 should be disconnected")
	}
}
*/

func TestPeerConcurrentHeaderCalls(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0 := blockchain.NewTestBlockchain(t, headers[0:5])

	// b1 with the whole chain
	b1 := blockchain.NewTestBlockchain(t, headers)

	eth0, _ := testEthProtocol(t, b0, b1)

	cases := []uint64{}
	for i := 10; i < 100; i++ {
		cases = append(cases, uint64(i))
	}

	errr := make(chan error, len(cases))

	for indx, i := range cases {
		go func(indx int, i uint64) {
			h, err := eth0.RequestHeadersSync(context.Background(), i, 100)
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
	b0 := blockchain.NewTestBlockchain(t, headers[0:5])

	// b1 with the whole chain
	b1 := blockchain.NewTestBlockchain(t, headers)

	eth0, _ := testEthProtocol(t, b0, b1)

	if _, err := eth0.RequestHeadersSync(context.Background(), 1100, 100); err == nil {
		t.Fatal("it should fail")
	}

	// NOTE: We cannot know from an empty response which is the
	// pending block it belongs to (because we use the first block to know the origin)
	// Thus, the query will fail with a timeout message but it does not
	// mean the peer is timing out on the responses.
}

func TestPeerContextCancel(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)

	// b0 with only the genesis
	b0 := blockchain.NewTestBlockchain(t, headers[0:5])

	// b1 with the whole chain
	b1 := blockchain.NewTestBlockchain(t, headers)

	eth0, _ := testEthProtocol(t, b0, b1)

	ctx, cancel := context.WithCancel(context.Background())

	errr := make(chan error, 1)
	go func() {
		_, err := eth0.RequestHeadersSync(ctx, 1100, 100)
		errr <- err
	}()

	cancel()
	if err := <-errr; err.Error() != "failed" {
		t.Fatal("it should have been canceled")
	}
}

func TestHeight(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(111)

	// b0 with only the genesis
	b0 := blockchain.NewTestBlockchain(t, headers)

	// b1 with the whole chain
	b1 := blockchain.NewTestBlockchain(t, headers)

	eth0, _ := ethPipe(b0, b1)
	height, err := eth0.fetchHeight(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if height.Number.Uint64() != 110 {
		t.Fatalf("it should be 110 but found %d", height.Number.Uint64())
	}
}

func TestPendingQueue(t *testing.T) {
	ack := make(chan AckMessage)
	c := &callback{ack}

	p := newPending()
	p.add("1", c)
	p.add("2", c)

	if p.pending != 2 {
		t.Fatal()
	}
	if _, ok := p.consume("1"); !ok {
		t.Fatal("failed to consume '1' the first time")
	}
	if _, ok := p.consume("1"); ok {
		t.Fatal("it consumed '1' twice")
	}
	if p.pending != 1 {
		t.Fatal()
	}

	// Consumes "2" because is the only one pending
	p.consume("")

	// Consume 2 should return false because has already been consumed
	if _, ok := p.consume("2"); ok {
		t.Fatal("'2' has already been consumed")
	}

	// Pending is empty, it should not consume more
	p.consume("")
	if p.pending != 0 {
		t.Fatal()
	}
}

func TestFailedQueries(t *testing.T) {
	upperBound := 100

	h0 := blockchain.NewTestHeaderChain(500)
	h1 := blockchain.NewTestHeaderChain(upperBound)

	b0 := blockchain.NewTestBlockchain(t, h0)
	b1 := blockchain.NewTestBlockchain(t, h1)

	eth0, _ := ethPipe(b0, b1)

	cases := [][]int{
		[]int{
			10, 200, 5,
		},
		[]int{
			5, 10, 200,
		},
		[]int{
			5, 200, 10,
		},
		[]int{
			200, 201, 202,
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			done := make(chan error, len(c))
			res := make([]error, len(c))

			for indx, step := range c {
				go func(indx, step int) {
					header, ok := b0.GetHeaderByNumber(big.NewInt(int64(step)))
					if !ok {
						done <- fmt.Errorf("header not found")
						return
					}

					_, err := eth0.RequestHeaderByHashSync(context.Background(), header.Hash())
					res[indx] = err

					done <- nil
				}(indx, step)
			}

			for i := 0; i < len(c); i++ {
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}

			for indx, step := range c {
				if step > upperBound && res[indx] == nil {
					t.Fatal()
				}
				if step < upperBound && res[indx] != nil {
					t.Fatal()
				}
			}
		})
	}
}
