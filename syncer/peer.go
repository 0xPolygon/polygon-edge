package syncer

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/protocol/ethereum"
)

const (
	receiptsContext = "receipts"
	bodiesContext   = "bodies"
	headersContext  = "headers"
)

// AckMessage is the ack message
type AckMessage struct {
	Complete bool
	Result   interface{}
}

type callback struct {
	id  uint32
	ack chan AckMessage
}

// Peer is a network connection in the syncer
type Peer struct {
	peer *network.Peer
	eth  *ethereum.Ethereum

	syncer *Syncer

	// workerPool chan chan Job
	closeCh chan struct{}
	once    sync.Once

	// pendin objects
	pending     map[string]*callback
	pendingLock sync.Mutex

	timer *time.Timer
}

// NewPeer creates a new peer
func NewPeer(peer *network.Peer, syncer *Syncer) *Peer {
	eth := peer.GetProtocol("eth", 63).(*ethereum.Ethereum)

	p := &Peer{
		peer:    peer,
		eth:     eth,
		closeCh: make(chan struct{}),
		// workerPool:  workerPool,
		syncer:      syncer,
		pending:     make(map[string]*callback),
		pendingLock: sync.Mutex{},
	}

	eth.SetDownloader(p)
	return p
}

func (p *Peer) requestTask(id string) {
	for {
		i := p.syncer.dequeue()
		if i == nil {
			time.Sleep(5 * time.Second) // if no job available, sleep and try again
			continue
		}

		var data interface{}
		var err error

		switch job := i.payload.(type) {
		case *HeadersJob:
			fmt.Printf("SYNC HEADERS (%d) (%s) (%s): %d, %d\n", i.id, p.peer.PrettyString(), id, job.block, job.count)
			data, err = p.requestHeaders(job.block, job.count)
		case *BodiesJob:
			fmt.Printf("SYNC BODIES (%d) (%s) (%s): %d\n", i.id, p.peer.PrettyString(), id, len(job.hashes))
			data, err = p.requestBodies(job.hashes)
		case *ReceiptsJob:
			fmt.Printf("SYNC RECEIPTS (%d) (%s) (%s): %d\n", i.id, p.peer.PrettyString(), id, len(job.hashes))
			data, err = p.requestReceipts(job.hashes)
		}

		p.syncer.deliver(i.id, data, err)

		time.Sleep(1 * time.Second)
	}
}

func (p *Peer) requestReceipts(receipts []*types.Header) ([][]*types.Receipt, error) {
	if len(receipts) == 0 {
		return nil, nil
	}

	hashes := []common.Hash{}
	for _, b := range receipts {
		hashes = append(hashes, b.Hash())
	}

	hash := receipts[0].ReceiptHash.String()

	ack := make(chan AckMessage, 1)
	p.setHandler(hash, 1, ack)

	if err := p.eth.RequestReceipts(hashes); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Complete {
		return nil, fmt.Errorf("failed")
	}

	// TODO. handle malformed response in the receipts
	response := resp.Result.([][]*types.Receipt)
	return response, nil
}

func (p *Peer) requestBodies(bodies []*types.Header) ([]*types.Body, error) {
	if len(bodies) == 0 {
		return nil, nil
	}

	hashes := []common.Hash{}
	for _, b := range bodies {
		hashes = append(hashes, b.Hash())
	}

	first := bodies[0]
	hash := encodeHash(first.UncleHash, first.TxHash).String()

	ack := make(chan AckMessage, 1)
	p.setHandler(hash, 1, ack)

	if err := p.eth.RequestBodies(hashes); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Complete {
		return nil, fmt.Errorf("failed")
	}

	// TODO. handle malformed response in the bodies
	response := resp.Result.(ethereum.BlockBodiesData)

	res := []*types.Body{}
	for _, r := range response {
		res = append(res, &types.Body{Transactions: r.Transactions, Uncles: r.Uncles})
	}

	return res, nil
}

func (p *Peer) requestHeaders(origin uint64, count uint64) ([]*types.Header, error) {
	hash := strconv.Itoa(int(origin))

	ack := make(chan AckMessage, 1)
	p.setHandler(hash, 1, ack)

	if err := p.eth.RequestHeadersByNumber(origin, count, 0, false); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Complete {
		return nil, fmt.Errorf("failed")
	}

	response := resp.Result.([]*types.Header)
	return response, nil
}

func (p *Peer) run(maxRequests int) {
	for i := 0; i < maxRequests; i++ {
		go p.requestTask(strconv.Itoa(i))
	}
}

// FindCommonAncestor finds the common ancestor with the peer and the syncer connection
func (p *Peer) FindCommonAncestor() (*types.Header, error) {
	// Binary search, TODO, works but it may take a lot of time

	min := 0 // genesis
	max := int(p.syncer.blockchain.Header().Number.Uint64())

	var header *types.Header

	for min <= max {
		m := uint64(math.Floor(float64(min+max) / 2))

		ack := make(chan network.AckMessage, 1)
		p.eth.Conn().SetHandler(ethereum.BlockHeadersMsg, ack, 5*time.Second)

		if err := p.eth.RequestHeadersByNumber(m, 1, 0, false); err != nil {
			return nil, err
		}

		resp := <-ack
		if !resp.Complete {
			return nil, fmt.Errorf("timeout")
		}

		var headers []*types.Header
		if err := rlp.DecodeBytes(resp.Payload, &headers); err != nil {
			return nil, err
		}

		l := len(headers)
		if l == 0 {
			// peer does not have the m peer, search in lower bounds
			max = int(m - 1)
		} else if l == 1 {
			header = headers[0]
			if header.Number.Uint64() != m {
				return nil, fmt.Errorf("header response number not correct, asked %d but retrieved %d", m, header.Number.Uint64())
			}

			expectedHeader := p.syncer.blockchain.GetHeaderByNumber(big.NewInt(int64(m)))
			if expectedHeader == nil {
				return nil, fmt.Errorf("cannot find the header in local chain")
			}

			if expectedHeader.Hash() == header.Hash() {
				min = int(m + 1)
			} else {
				max = int(m - 1)
			}
		} else {
			return nil, fmt.Errorf("expected either 1 or 0 headers")
		}
	}

	if min == 0 {
		return nil, nil
	}
	return header, nil
}

// fetchHeight returns the header of the head hash of the peer
func (p *Peer) fetchHeight() (*types.Header, error) {
	head := p.peer.HeaderHash()

	ack := make(chan network.AckMessage, 1)
	p.eth.Conn().SetHandler(ethereum.BlockHeadersMsg, ack, 10*time.Second)

	if err := p.eth.RequestHeadersByHash(head, 1, 0, false); err != nil {
		return nil, err
	}

	resp := <-ack
	if !resp.Complete {
		return nil, fmt.Errorf("timeout")
	}

	var headers []*types.Header
	if err := rlp.DecodeBytes(resp.Payload, &headers); err != nil {
		return nil, err
	}
	if len(headers) != 1 {
		return nil, fmt.Errorf("expected one but found %d", len(headers))
	}

	header := headers[0]
	if header.Hash() != head {
		return nil, fmt.Errorf("returned hash is not the correct one")
	}

	return header, nil
}

// -- handlers --

func (p *Peer) setHandler(key string, id uint32, ack chan AckMessage) error {
	p.pendingLock.Lock()
	p.pending[key] = &callback{id, ack}
	p.pendingLock.Unlock()

	p.timer = time.AfterFunc(5*time.Second, func() {
		p.pendingLock.Lock()
		if _, ok := p.pending[key]; !ok {
			p.pendingLock.Unlock()
			return
		}

		delete(p.pending, key)
		p.pendingLock.Unlock()

		select {
		case ack <- AckMessage{false, nil}:
		default:
		}
	})

	return nil
}

func (p *Peer) consumeHandler(origin string, result interface{}) bool {
	p.pendingLock.Lock()
	callback, ok := p.pending[origin]
	if !ok {
		p.pendingLock.Unlock()
		return false
	}

	// delete
	delete(p.pending, origin)
	p.pendingLock.Unlock()

	// let him know its over
	select {
	case callback.ack <- AckMessage{Complete: true, Result: result}:
	default:
	}

	return true
}

// -- downloader --

// Headers receives the headers
func (p *Peer) Headers(headers []*types.Header) {
	if len(headers) == 0 {
		// The peer did not have the headers of the query,
		// we cannot know to which pending query it belongs
		return
	}

	hash := headers[0].Number.String()
	if !p.consumeHandler(hash, headers) {
		fmt.Println("Could not consume headers handler")
	}
}

// Receipts receives the receipts
func (p *Peer) Receipts(receipts [][]*types.Receipt) {
	if len(receipts) == 0 {
		// this should only happen if the other peer does not have the data
		return
	}

	hash := types.DeriveSha(types.Receipts(receipts[0]))
	if !p.consumeHandler(hash.String(), receipts) {
		fmt.Println("Could not consume receipts handler")
	}
}

// Bodies receives the bodies
func (p *Peer) Bodies(bodies ethereum.BlockBodiesData) {
	if len(bodies) == 0 {
		// this should only happen if the other peer does not have the data
		return
	}

	first := bodies[0]
	hash := encodeHash(types.CalcUncleHash(first.Uncles), types.DeriveSha(types.Transactions(first.Transactions)))

	if !p.consumeHandler(hash.String(), bodies) {
		fmt.Println("Could not consume bodies handler")
	}
}

func encodeHash(x common.Hash, y common.Hash) common.Hash {
	hw := sha3.NewKeccak256()
	if _, err := hw.Write(x.Bytes()); err != nil {
		panic(err)
	}
	if _, err := hw.Write(y.Bytes()); err != nil {
		panic(err)
	}

	var h common.Hash
	hw.Sum(h[:0])
	return h
}
