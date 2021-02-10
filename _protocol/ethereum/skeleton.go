package ethereum

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/types"
	"github.com/0xPolygon/minimal/types/buildroot"
	"github.com/umbracle/fastrlp"
)

const skeletonSize = 190

func newSlot() *Slot {
	return &Slot{
		size:    190,
		headers: make([]types.Header, 190),
		status: map[RequestType]RequestStatus{ // TODO, we can do better than this
			Headers:  Pending,
			Bodies:   Completed,
			Receipts: Completed,
		},
	}
}

// Slot is the slots in the epoch, groups of 190
type Slot struct {
	sync.Mutex
	indx uint64

	// sentinel value from the skeleton
	hash   []byte
	number uint64

	// completed is bool if the slot does not need more requests
	completed bool

	// list of headers in the slot
	headers []types.Header
	size    uint64

	// state of the queries
	status map[RequestType]RequestStatus

	// index of the headers
	bodiesIndx   uint64
	receiptsIndx uint64

	// list of transactions in the slot
	txnsDelta []uint64
	txns      []*types.Transaction
	txnsPtr   int

	// list of uncles in the slot
	unclesDelta []uint64
	uncles      []*types.Header
	unclesPtr   int

	// list of receipts in the slot
	receiptsDelta []uint64
	receipts      []*types.Receipt
	receiptsPtr   int
}

// Len is the size of the slot
func (s *Slot) Len() uint64 {
	return s.size
}

func (s *Slot) reset() {
	s.indx = 0
	s.completed = false

	// s.size = 0

	s.bodiesIndx = 0
	s.receiptsIndx = 0

	// reset header status
	s.status[Headers] = Pending

	s.txnsDelta = s.txnsDelta[:0]
	// s.txns = s.txns[:0]
	s.txnsPtr = 0

	s.unclesDelta = s.unclesDelta[:0]
	// s.uncles = s.uncles[:0]
	s.unclesPtr = 0

	s.receiptsDelta = s.receiptsDelta[:0]
	// s.receipts = s.receipts[:0]
	s.receiptsPtr = 0
}

// Deliver delivers a response
func (s *Slot) Deliver(q *Queue3, req *Request, p *fastrlp.Parser, v *fastrlp.Value, isLast bool) error {
	s.Lock()
	defer s.Unlock()

	if s.status[req.typ] == Completed {
		// Delivering something on the completed slot
		return nil
	}

	switch req.typ {
	case Headers:
		// We dont need to take snapshot of the headers because the header entries in the slot are
		// filled in one 'shot'. Thus, if it fails and the slots gets half filled with headers the next
		// response for headers will fill those entries again starting with the index 0.
		return s.deliverHeaders(q, req, p, v, isLast)
	case Bodies:
		reset := s.takeBodiesSnapshot()
		if err := s.deliverBodies(req, p, v); err != nil {
			reset()
			return err
		}
	case Receipts:
		reset := s.takeReceiptsSnapshot()
		if err := s.deliverReceipts(req, p, v); err != nil {
			reset()
			return err
		}
	}

	// check if the slot is fully completed
	if s.status[Headers] == Completed && s.status[Receipts] == Completed && s.status[Bodies] == Completed {
		s.completed = true
	}
	return nil
}

func (s *Slot) deliverHeaders(q *Queue3, req *Request, p *fastrlp.Parser, v *fastrlp.Value, isLast bool) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	num := len(elems)
	if !isLast && uint64(num) != s.size {
		return fmt.Errorf("bad number of headers delivered")
	}

	// decode the first header to check if it matches with the beacon
	if err := s.headers[0].UnmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}
	// validate the beacon hash
	if !bytes.Equal(s.headers[0].Hash.Bytes(), s.hash) {
		return fmt.Errorf("not valid hash")
	}
	// validate the beacon number
	if s.headers[0].Number != s.number {
		return fmt.Errorf("number not correct")
	}

	// Unmarshal the rest of the headers
	parent := s.headers[0]
	for i := 1; i < num; i++ {
		if err := s.headers[i].UnmarshalRLPFrom(p, elems[i]); err != nil {
			return err
		}
		// validate the sequential numbers
		if s.headers[i].Number-1 != parent.Number {
			return fmt.Errorf("numbers not sequential")
		}
		// validate the chain of blocks
		if s.headers[i].ParentHash != parent.Hash {
			return fmt.Errorf("bad")
		}
		parent = s.headers[i]
	}

	// TODO, do better
	s.setStatus(Headers, Completed)

	// check the new headers and see if there is any bodies or receipts to query next
	var receipts, bodies bool
	for i := 0; i < num; i++ {
		header := s.headers[i]
		if q.downloadBodies { // same line
			if hasBody(header) {
				bodies = true
			}
		}
		if q.downloadReceipts { // same line
			if hasReceipts(header) {
				receipts = true
			}
		}
	}

	if bodies {
		s.setStatus(Bodies, Pending)
	} else {
		s.fillBodiesDelta()
	}

	if receipts {
		s.setStatus(Receipts, Pending)
	} else {
		s.fillReceiptsDelta()
	}

	// Everything is completed now
	if !receipts && !bodies {
		s.completed = true
	}
	return nil
}

// TODO, test
func (s *Slot) fillBodiesDelta() {
	for i := 0; i < 190; i++ {
		s.unclesDelta = append(s.unclesDelta, 0)
		s.txnsDelta = append(s.txnsDelta, 0)
	}
}

func (s *Slot) fillReceiptsDelta() {
	for i := 0; i < 190; i++ {
		s.receiptsDelta = append(s.receiptsDelta, 0)
	}
}

func (s *Slot) takeReceiptsSnapshot() func() {
	// l0 := len(s.receipts)
	l0 := s.receiptsPtr

	return func() {
		// reset receipts
		if l0 != s.receiptsPtr {
			s.receiptsPtr = l0
		}
		s.receiptsDelta = s.receiptsDelta[:s.receiptsIndx]
	}
}

func (s *Slot) takeBodiesSnapshot() func() {
	l0 := s.unclesPtr
	l1 := s.txnsPtr

	return func() {
		// reset uncles
		if l0 != s.unclesPtr {
			s.unclesPtr = l0
		}
		s.unclesDelta = s.unclesDelta[:s.bodiesIndx]

		// reset transactions
		if l1 != s.txnsPtr {
			s.txnsPtr = l1
		}
		s.txnsDelta = s.txnsDelta[:s.bodiesIndx]
	}
}

func (s *Slot) getReceipt() *types.Receipt {
	if s.receiptsPtr == len(s.receipts) {
		s.receipts = append(s.receipts, &types.Receipt{})
	}
	s.receiptsPtr++
	return s.receipts[s.receiptsPtr-1]
}

func (s *Slot) getUncle() *types.Header {
	if s.unclesPtr == len(s.uncles) {
		s.uncles = append(s.uncles, &types.Header{})
	}
	s.unclesPtr++
	return s.uncles[s.unclesPtr-1]
}

func (s *Slot) getTxn() *types.Transaction {
	if s.txnsPtr == len(s.txns) {
		s.txns = append(s.txns, &types.Transaction{})
	}
	s.txnsPtr++
	return s.txns[s.txnsPtr-1]
}

// var kpool keccakPool

func (s *Slot) deliverBodies(req *Request, p *fastrlp.Parser, v *fastrlp.Value) error {
	// Validate root data
	elems, err := v.GetElems()
	if err != nil {
		panic(err)
	}

	// the number of elements must be at most the number of items requested
	if len(elems) > int(req.count) {
		return fmt.Errorf("too many elements")
	}

	// check the index in the response has to be the same one as the current index
	// otherwise this may be an out-of-order response
	if req.indx != s.bodiesIndx {
		return fmt.Errorf("bad index")
	}

	indx := s.bodiesIndx
	for _, body := range elems {
		// bound check on the body
		if body.Type() != fastrlp.TypeArray {
			panic("is not an array")
		}
		if body.Elems() != 2 {
			panic("expected two values")
		}

		// get the next valid header
		for !hasBody(s.headers[indx]) {
			indx++

			// increase deltas for a block without uncles nor headers
			s.unclesDelta = append(s.unclesDelta, 0)
			s.txnsDelta = append(s.txnsDelta, 0)

			if indx > s.Len() {
				// TODO, change this loop to return an internal error instead of panic
				// This should never happen, we got at most the count elements in the response (otherwise it fails the previous check)
				// That count is made by checking hasBody on the headers. Thus, this iterates over all the valid headers
				panic("internal")
			}
		}

		// header should match body
		header := s.headers[indx]
		indx++

		// check transactions in index 0
		txns, err := body.Get(0).GetElems()
		if err != nil {
			panic(err)
		}
		if len(txns) == 0 {
			if header.TxRoot != types.EmptyRootHash {
				panic("bad.1")
			}
			s.txnsDelta = append(s.txnsDelta, 0)
		} else {
			// derive root of the transactions
			if txRoot := calculateRoot(p, txns, types.EmptyRootHash); txRoot != header.TxRoot {
				return fmt.Errorf("txroot is different")
			}

			for _, elem := range txns {
				txn := s.getTxn()
				if err := txn.UnmarshalRLPFrom(p, elem); err != nil {
					return err
				}
			}

			// add transactions delta values
			s.txnsDelta = append(s.txnsDelta, uint64(len(txns)))
		}

		// check uncles in index 1
		uncles, err := body.Get(1).GetElems()
		if err != nil {
			panic(err)
		}
		if len(uncles) == 0 {
			if header.Sha3Uncles != types.EmptyUncleHash {
				panic("bad")
			}
			s.unclesDelta = append(s.unclesDelta, 0)
		} else {
			var unclesRoot types.Hash

			k := keccak.DefaultKeccakPool.Get()
			k.Write(p.Raw(body.Get(1)))
			k.Sum(unclesRoot[:0])
			keccak.DefaultKeccakPool.Put(k)

			// derive root of the uncles
			if unclesRoot != header.Sha3Uncles {
				return fmt.Errorf("uncle root is different")
			}

			for _, elem := range uncles {
				uncle := s.getUncle()
				if err := uncle.UnmarshalRLPFrom(p, elem); err != nil {
					return err
				}
			}

			// add uncles delta values
			s.unclesDelta = append(s.unclesDelta, uint64(len(uncles)))
		}
	}

	// Advance the index to see if the last elements dont have anything, TODO: TEST
	for indx < s.Len() && !hasBody(s.headers[indx]) {
		s.unclesDelta = append(s.unclesDelta, 0)
		s.txnsDelta = append(s.txnsDelta, 0)
		indx++
	}

	// everything was included correctly, replace the bodies index for the next query
	s.bodiesIndx = indx

	// set the entry as completed if there are no more entries to fetch
	if indx == s.Len() {
		s.setStatus(Bodies, Completed)
	} else {
		s.setStatus(Bodies, Pending)
	}

	return nil
}

func (s *Slot) setStatus(req RequestType, status RequestStatus) {
	// fmt.Printf("Set Status (%d) (%s) => (%s)\n", s.indx, req.String(), status.String())
	s.status[req] = status
}

func (s *Slot) deliverReceipts(req *Request, p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		panic(err)
	}

	indx := s.receiptsIndx
	for _, item := range elems {
		// get the next valid header with receipts
		for !hasReceipts(s.headers[indx]) {
			s.receiptsDelta = append(s.receiptsDelta, 0)

			indx++
			if indx > s.Len() {
				// This should never happen, we got at most the count elements in the response (otherwise it fails the previous check)
				// That count is made by checking hasBody on the headers. Thus, this iterates over all the valid headers
				panic("internal")
			}
		}

		// next valid header
		header := s.headers[indx]
		indx++

		receipts, err := item.GetElems()
		if err != nil {
			panic(err)
		}

		if len(receipts) == 0 {
			// can we actually check for empty receipts, no!!
			if header.ReceiptsRoot != types.EmptyRootHash {
				panic("bad")
			}
			s.receiptsDelta = append(s.receiptsDelta, 0)
		} else {
			// derive receipts root
			if root := calculateRoot(p, receipts, types.EmptyRootHash); root != header.ReceiptsRoot {
				return fmt.Errorf("bad receipts root")
			}
			for _, elem := range receipts {
				receipt := s.getReceipt()
				if err := receipt.UnmarshalRLPFrom(p, elem); err != nil {
					return err
				}
			}
			s.receiptsDelta = append(s.receiptsDelta, uint64(len(receipts)))
		}
	}

	// Advance the index to see if the last elements dont have anything, TODO: TEST
	for indx < s.Len() && !hasReceipts(s.headers[indx]) {
		s.receiptsDelta = append(s.receiptsDelta, 0)
		indx++
	}

	// everything was included correctly, replace the bodies index for the next query
	s.receiptsIndx = indx

	// set the entry as completed if there are no more entries to fetch
	if indx == s.Len() {
		s.setStatus(Receipts, Completed)
	} else {
		s.setStatus(Receipts, Pending)
	}

	return nil
}

// GetJob returns a job from this slot
func (s *Slot) GetJob(indx uint, req *Request) bool {
	if s.status[Headers] == Pending {
		req.typ = Headers
		req.buf = reqHeadersQuery(req.buf[:0], s.number, 190, 0, false) // the -1 may break the tests
		req.origin = strconv.Itoa(int(s.number))
		req.count = 190 // change later

		s.setStatus(Headers, OnFly)
		return true
	} else if s.status[Bodies] == Pending {
		req.typ = Bodies
		req.buf, req.origin, req.count = s.getHashesQuery(req.buf[:0], s.bodiesIndx, true)
		req.indx = s.bodiesIndx

		s.setStatus(Bodies, OnFly)
		return true

	} else if s.status[Receipts] == Pending {

		req.typ = Receipts
		req.buf, req.origin, req.count = s.getHashesQuery(req.buf[:0], s.receiptsIndx, false)
		req.indx = s.receiptsIndx

		s.setStatus(Receipts, OnFly)
		return true
	}

	return false
}

var reqHashesPool fastrlp.ArenaPool

func (s *Slot) getHashesQuery(dst []byte, indx uint64, bodies bool) ([]byte, string, uint64) {
	a := reqHashesPool.Get()

	v := a.NewArray()
	count := uint64(0)

	var origin []byte
	for i := indx; i < uint64(s.Len()); i++ {
		var buf []byte

		header := s.headers[i]
		if bodies && hasBody(header) {
			buf = header.Hash.Bytes()
		}
		if !bodies && hasReceipts(header) {
			buf = header.Hash.Bytes()
		}
		if buf != nil {
			if count == 0 {
				// use this value as beacon origin
				if bodies {
					// xor of txnRoot and sha3Uncles
					origin = xor(header.TxRoot, header.Sha3Uncles)
				} else {
					origin = header.ReceiptsRoot.Bytes()
				}
			}
			count++
			v.Set(a.NewBytes(buf))
		}
	}

	dst = v.MarshalTo(dst)
	reqHashesPool.Put(a)
	return dst, hex.EncodeToHex(origin), count
}

var reqHeadersPool fastrlp.ArenaPool

func reqHeadersQuery(dst []byte, from, amount, skip uint64, reverse bool) []byte {
	a := reqHeadersPool.Get()

	v := a.NewArray()
	v.Set(a.NewUint(from))    // origin is a number
	v.Set(a.NewUint(amount))  // amount
	v.Set(a.NewUint(skip))    // skip
	v.Set(a.NewBool(reverse)) // reverse

	dst = v.MarshalTo(dst)
	reqHeadersPool.Put(a)
	return dst
}

// Queue3 stores the data in epochs, one epoch at a time, only when one epoch is consumed
// we proceed to consume the next one
type Queue3 struct {
	// seq is used to determine out of place requests
	seq uint64

	// slots is a list to store slots
	slots     []*Slot
	slotsLock sync.Mutex

	// numSlots is the number of valid slots available
	numSlots int

	// completed true if the queue is fully filled
	completed bool

	// numCompleted is the number of completed slots
	numCompleted int

	// indx is the index of the first non finished slot
	indx int

	// This flags indicate if we have to download bodies and receipts in the queue
	downloadBodies   bool
	downloadReceipts bool

	advanceCh chan struct{}
	doneCh    chan struct{}
	readyCh   chan struct{} // how to use this to notify that the skeleton is now ready, closing the channel maybe

	// iterator fields
	iterIndx int
	itemIndx int64

	// cummulative iterators for the slots
	txnCum      uint64
	receiptsCum uint64
	unclesCum   uint64

	block *types.Block
}

// Wait is to wait for the data to be ready
func (q *Queue3) Wait() {
	q.readyCh = make(chan struct{}, 1)
}

func (q *Queue3) reset() {
	// reset all the slots
	for _, slot := range q.slots {
		slot.reset()
	}

	q.completed = false
	q.numCompleted = 0

	q.indx = 0
	q.iterIndx = 0
	q.itemIndx = 0

	// flush the advance channel
	for {
		select {
		case <-q.advanceCh:
		default:
			goto EXIT
		}
	}
EXIT:
}

func (q *Queue3) notifyQueue() {
	select {
	case q.advanceCh <- struct{}{}:
	default:
	}
}

// IsCompleted returns true if the queue is completed
func (q *Queue3) IsCompleted() bool {
	return q.indx == q.Len()
}

func (q *Queue3) ReassignJob(req *Request) {
	slotIndx := int(req.slot)

	q.slotsLock.Lock()
	slot := q.slots[slotIndx]
	q.slotsLock.Unlock()

	if req.typ == Headers {
		slot.setStatus(Headers, Pending)
	}
	if req.typ == Bodies {
		slot.setStatus(Bodies, Pending)
	}
	if req.typ == Receipts {
		slot.setStatus(Receipts, Pending)
	}
}

// Head returns the head of the skeleton
func (q *Queue3) Head() *types.Header {
	slot := q.slots[q.Len()-1]
	return &slot.headers[slot.Len()-1]
}

func (q *Queue3) Len() int {
	return q.numSlots
}

// DeliverJob delivers a job
func (q *Queue3) DeliverJob(req *Request, p *fastrlp.Parser, v *fastrlp.Value) error {
	// TODO, check if the skeleton is not ready yet, q.readyCh is activated
	if req.typ == NodeData {
		panic("internal. queue does not handle node data requests")
	}
	if q.seq != req.seq {
		// response out of sequence
		return nil
	}

	slotIndx := int(req.slot)

	q.slotsLock.Lock() // not sure if this one is necessary or not
	slot := q.slots[slotIndx]
	q.slotsLock.Unlock()

	isLast := (slotIndx + 1) == q.Len()
	if err := slot.Deliver(q, req, p, v, isLast); err != nil {
		return err
	}

	var advanced bool
	if slot.completed {
		q.numCompleted++

		fmt.Printf("slot %d completed\n", slotIndx)
		//fmt.Println(slotIndx, q.indx)

		if slotIndx < q.indx {
			panic("cannot finish a slot with the index advanced")
		}

		// increase indx (can we do this without a lock?)
		// need to see which is the current index and the position of this slot
		if slotIndx == q.indx {
			// Advance all the completed slots in case we just finished this one out of order
			for q.indx < q.Len() && q.slots[q.indx].completed {
				advanced = true
				q.indx++
			}
			// If there are no more slots to complete the skeleton is completed
			if q.indx == q.Len() {
				q.completed = true
			}
		}
	}

	if advanced {
		q.notifyQueue()
	}
	return nil
}

// GetJob gets a new job from the queue
func (q *Queue3) GetJob(req *Request) bool {
	// wait in case its not ready yet
	if q.readyCh != nil {
		<-q.readyCh
	}
	if q.IsCompleted() {
		return false
	}

	// check if there are too many pending slots
	// We also need to consider the reading pointer

	req.seq = q.seq
	for i := q.indx; i < q.Len(); i++ {
		slot := q.slots[i]
		slot.Lock()
		ok := slot.GetJob(uint(i), req)
		slot.Unlock()

		if ok {
			req.slot = uint64(i)
			return true
		}
	}

	// no job found
	return false
}

func (q *Queue3) resetIteratorSlot() {
	q.itemIndx = 0
	q.txnCum = 0
	q.unclesCum = 0
	q.receiptsCum = 0
}

func (q *Queue3) Next() *types.Block {
	if q.block == nil {
		q.block = new(types.Block)
	}

	if q.iterIndx == q.indx {
		if q.indx == q.Len() && q.indx != 0 {
			// send a message to doneCh
			q.doneCh <- struct{}{}

			// so that we have some sort of blocking mechanism here
			q.Wait()

			<-q.readyCh
		}
		// wait till this slot is finished
		<-q.advanceCh
	}

	// dont need to lock because that slot wont be access anymore
	slot := q.slots[q.iterIndx]

	// transactions of the block
	numTxns := slot.txnsDelta[q.itemIndx]
	txns := slot.txns[q.txnCum : q.txnCum+numTxns]
	q.txnCum += numTxns

	// uncles of the block
	numUncles := slot.unclesDelta[q.itemIndx]
	uncles := slot.uncles[q.unclesCum : q.unclesCum+numUncles]
	q.unclesCum += numUncles

	q.block.Header = &slot.headers[q.itemIndx]
	q.block.Transactions = txns
	q.block.Uncles = uncles

	q.itemIndx++
	if q.itemIndx == int64(slot.Len()) {
		q.iterIndx++
		q.resetIteratorSlot()
	}

	return q.block
}

func (q *Queue3) incSeq() {
	atomic.AddUint64(&q.seq, 1)
}

// AddSkeleton adds a skeleton to the queue
func (q *Queue3) AddSkeleton(headers []*types.Header, lastSlotNum uint64) error {
	if len(headers) == 0 {
		return fmt.Errorf("empty headers")
	}
	if len(headers) > 190 {
		return fmt.Errorf("higher than 190")
	}

	// Validate the sequence of numbers
	for i := 1; i < len(headers)-1; i += 2 {
		diff := headers[i+1].Number - headers[i].Number
		if diff != 190 {
			return fmt.Errorf("headers diff not correct, expected %d but found %d", 190, diff)
		}
	}

	q.reset()
	q.numSlots = len(headers)

	// create extra headers if needed
	if len(q.slots) < len(headers) {
		aux := []*Slot{}
		for i := 0; i < len(headers)-len(q.slots); i++ {
			slot := newSlot()
			slot.indx = uint64(len(q.slots) + i)
			aux = append(aux, slot)
		}
		q.slots = append(q.slots, aux...)
	}

	// increase the sequence, past queries are not valid anymore
	q.incSeq()

	// Update the skeleton entries
	for indx, i := range headers {
		q.slots[indx].number = i.Number
		q.slots[indx].hash = append(q.slots[indx].hash[:0], i.Hash.Bytes()...)

		if indx != q.numSlots-1 {
			q.slots[indx].size = 190 // 189 + beacon
		} else {
			q.slots[indx].size = 1 + lastSlotNum
		}
	}

	// it should be always open at this point
	if q.readyCh != nil {
		close(q.readyCh)
	}

	return nil
}

// RequestStatus is the status of the items in the slot (RENAME)
type RequestStatus uint

const (
	// Completed if the item has been fully downloaded
	Completed RequestStatus = iota
	// OnFly if the item is being requested
	OnFly
	// Pending if the item has to be scheduled for downloading
	Pending
)

func (r RequestStatus) String() string {
	switch r {
	case Completed:
		return "completed"
	case OnFly:
		return "onfly"
	case Pending:
		return "pending"
	}
	return ""
}

type RequestType uint

const (
	Headers RequestType = iota
	Bodies
	Receipts
	NodeData
)

func (r *RequestType) String() string {
	switch *r {
	case Headers:
		return "headers"
	case Bodies:
		return "bodies"
	case Receipts:
		return "receipts"
	case NodeData:
		return "nodedata"
	}
	return ""
}

// Request is a request from the skeleton queue to retrieve information
type Request struct {
	// indx of the item you are requesting, used to find out-of-order responses
	indx uint64
	// seq id of the current skeleton. Used to find responses from different skeletons.
	seq uint64
	// slot in the skeleton to which the request belongs
	slot uint64
	// number of items requested (used by bodies and receipts)
	count uint64
	// origin id for the setHandler function
	origin string
	// type of request (headers, bodies or receipts)
	typ RequestType
	// buffer with the raw message
	buf []byte
}

func hasBody(h types.Header) bool {
	return h.TxRoot != types.EmptyRootHash || h.Sha3Uncles != types.EmptyUncleHash
}

func hasReceipts(h types.Header) bool {
	return h.ReceiptsRoot != types.EmptyRootHash
}

func calculateRoot(p *fastrlp.Parser, elems []*fastrlp.Value, def types.Hash) types.Hash {
	num := len(elems)
	if num == 0 {
		return def
	}

	return buildroot.CalculateRoot(num, func(i int) []byte {
		return p.Raw(elems[i])
	})

	/*
		ar := &fastrlp.Arena{}
		t := itrie.NewTrie()
		txn := t.Txn()

		for indx, elem := range elems {
			j := ar.NewUint(uint64(indx))
			txn.Insert(j.MarshalTo(nil), p.Raw(elem))
		}

		root, _ := txn.Hash()
		return types.BytesToHash(root)
	*/

}
