package ethereum

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"reflect"
	"testing"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/network/transport/rlpx"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/fastrlp"
)

func newTestEthereumProto(peerID string, conn net.Conn, b *blockchain.Blockchain) *Ethereum {
	logger := hclog.New(&hclog.LoggerOptions{
		Output: ioutil.Discard,
	})

	if peerID == "" {
		peerID = "1"
	}
	return NewEthereumProtocol(&mockSession{}, peerID, logger, conn, b)
}

var status = Status{
	ProtocolVersion: 63,
	NetworkID:       1,
	TD:              big.NewInt(1),
	CurrentBlock:    types.StringToHash("1"),
	GenesisBlock:    types.StringToHash("1"),
}

func TestHandshake(t *testing.T) {
	// Networkid is different
	status1 := status
	status1.NetworkID = 2

	// Current block is different
	status2 := status
	status2.CurrentBlock = types.StringToHash("2")

	// Genesis block is different
	status3 := status
	status3.GenesisBlock = types.StringToHash("2")

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

	eth0 := newTestEthereumProto("", conn0, b0)
	eth1 := newTestEthereumProto("", conn1, b1)

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
		n = append(n, int(h.Number))
	}
	return n
}

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
			h, err := eth0.requestHeaderByNumber2(i, nil, 100, 0, false)
			if err == nil {
				if len(h) != 100 {
					err = fmt.Errorf("length not correct")
				} else {
					for indx, j := range h {
						if j.Number != i+uint64(indx) {
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

func TestHeight(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(111)

	// b0 with only the genesis
	b0 := blockchain.NewTestBlockchain(t, headers)

	// b1 with the whole chain
	b1 := blockchain.NewTestBlockchain(t, headers)

	eth0, _ := ethPipe(b0, b1)
	height, err := eth0.fetchHeight2()
	if err != nil {
		t.Fatal(err)
	}
	if height.Number != 110 {
		t.Fatalf("it should be 110 but found %d", height.Number)
	}
}

func TestPendingQueue(t *testing.T) {
	ack := make(chan AckMessage)
	c := &callback{ack: ack}

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

func pipeMock() (*mockSession, *mockSession) {
	closeCh := make(chan struct{})

	conn0, conn1 := net.Pipe()
	return &mockSession{closeCh, conn0}, &mockSession{closeCh, conn1}
}

type mockSession struct {
	closeCh chan struct{}
	conn    net.Conn
}

func (m *mockSession) Streams() []network.Stream {
	return nil
}

func (m *mockSession) GetInfo() network.Info {
	return network.Info{}
}

func (m *mockSession) CloseChan() <-chan struct{} {
	return m.closeCh
}

func (m *mockSession) IsClosed() bool {
	if m.closeCh == nil {
		return false
	}
	select {
	case <-m.closeCh:
		return true
	default:
		return false
	}
}

func (m *mockSession) Close() error {
	close(m.closeCh)
	m.conn.Close()
	return nil
}

func readRlpxMsg(t *testing.T, r io.Reader) <-chan []byte {
	res := make(chan []byte)
	go func() {
		for {
			// read header
			var recv rlpx.Header
			recv = make([]byte, rlpx.HeaderSize)
			if _, err := r.Read(recv); err != nil {
				t.Fatal(err)
			}
			// read the content
			buf := make([]byte, recv.Length())
			if _, err := r.Read(buf); err != nil {
				t.Fatal(err)
			}
			res <- buf
		}
	}()
	return res
}

func parse(t *testing.T, query []byte) (*fastrlp.Parser, *fastrlp.Value) {
	p := &fastrlp.Parser{}
	v, err := p.Parse(query)
	if err != nil {
		t.Fatal(err)
	}
	return p, v
}

func TestHandleGetBlockHeaders(t *testing.T) {
	p0, p1 := net.Pipe()

	chain := blockchain.NewTestBlockchain(t, blockchain.NewTestHeaderChain(100))
	e := &Ethereum{
		blockchain: chain,
		conn:       p1,
	}

	resp := readRlpxMsg(t, p0)

	cases := []struct {
		Origin   uint64
		Amount   uint64
		Skip     uint64
		Reverse  bool
		Expected []uint64
	}{
		{
			1, 10, 0, false,
			[]uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		{
			10, 10, 0, true,
			[]uint64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		},
		{
			1, 10, 4, false,
			[]uint64{1, 6, 11, 16, 21, 26, 31, 36, 41, 46},
		},
		{
			1, 1, 0, false,
			[]uint64{1},
		},
		{
			10000, 10, 0, false,
			[]uint64{},
		},
		{
			98, 10, 0, false,
			[]uint64{98, 99},
		},
	}

	for _, i := range cases {
		t.Run("", func(t *testing.T) {
			query := reqHeadersQuery(nil, i.Origin, i.Amount, i.Skip, i.Reverse)
			if err := e.handleGetHeader(parse(t, query)); err != nil {
				t.Fatal(err)
			}

			buf0 := <-resp

			_, v := parse(t, buf0)
			elems, _ := v.GetElems()
			assert.Equal(t, len(i.Expected), len(elems))

			headers := make([]types.Header, len(elems))
			for indx, header := range headers {
				if err := header.UnmarshalRLP(nil, elems[indx]); err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, header.Number, i.Expected[indx])
			}
		})
	}
}
