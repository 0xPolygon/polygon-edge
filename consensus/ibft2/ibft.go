package ibft2

import (
	"container/heap"
	"context"
	"crypto/ecdsa"
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/txpool"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

type blockchainInterface interface {
	Header() *types.Header
}

type Ibft2 struct {
	logger hclog.Logger
	config *consensus.Config
	index  uint64

	state  uint64
	state2 *currentState

	blockchain blockchainInterface
	executor   *state.Executor
	closeCh    chan struct{}

	validatorKey     *ecdsa.PrivateKey
	validatorKeyAddr types.Address

	preprepareCh chan *proto.MessageReq
	commitCh     chan *proto.MessageReq
	prepareCh    chan *proto.MessageReq

	// We use a message queue to process messages in order
	msgQueueLock sync.Mutex
	msgQueue     msgQueueImpl
	updateCh     chan struct{}

	// TODOOOO: We take this info from the blockchain
	validators []types.Address

	network *network.Server

	transportFactory transportFactory
	transport        transport
}

func Factory(ctx context.Context, config *consensus.Config, txpool *txpool.TxPool, blockchain *blockchain.Blockchain, executor *state.Executor, privateKey *ecdsa.PrivateKey, logger hclog.Logger) (consensus.Consensus, error) {
	p := &Ibft2{
		logger:           logger.Named("ibft2"),
		config:           config,
		blockchain:       blockchain,
		executor:         executor,
		closeCh:          make(chan struct{}),
		transportFactory: grpcTransportFactory,
		state2:           &currentState{},
		preprepareCh:     make(chan *proto.MessageReq, 10),
		commitCh:         make(chan *proto.MessageReq, 10),
		prepareCh:        make(chan *proto.MessageReq, 10),
	}
	p.createKey()
	return p, nil
}

func (i *Ibft2) createKey() error {
	i.msgQueue = msgQueueImpl{}
	i.updateCh = make(chan struct{})

	// generate a validator private key
	validatorKey, err := crypto.ReadPrivKey(filepath.Join(i.config.Path, "validator.key"))
	if err != nil {
		return err
	}
	i.validatorKey = validatorKey
	i.validatorKeyAddr = crypto.PubKeyToAddress(&validatorKey.PublicKey)
	return nil
}

type IbftState uint32

const (
	AcceptState IbftState = iota
	RoundChangeState
	PreprepareState // TODO: REMOVE

	ValidateState

	// Combine
	PrepareState
	CommitState
)

func (i IbftState) String() string {
	switch i {
	case AcceptState:
		return "AcceptState"

	case RoundChangeState:
		return "RoundChangeState"

	case PreprepareState:
		return "PreprepareState"

	case ValidateState:
		return "ValidateState"

	case PrepareState:
		return "PrepareState"

	case CommitState:
		return "CommitState"
	}
	panic(fmt.Sprintf("BUG: Ibft state not found %d", i))
}

func (i *Ibft2) readMessages() {
	ch := i.transport.Listen()
	for {
		msg := <-ch

		switch msg.Message.(type) {
		case *proto.MessageReq_Preprepare:
			i.preprepareCh <- msg

		case *proto.MessageReq_Prepare:
			i.prepareCh <- msg

		case *proto.MessageReq_Commit:
			i.commitCh <- msg

		default:
			panic("TODO MESSAGE")
		}
	}
}

func (i *Ibft2) start() {
	go i.readMessages()

	for {
		select {
		case <-i.closeCh:
			return
		default:
		}

		switch i.getState() {
		case AcceptState:
			i.runAcceptState()

		case ValidateState:
			i.runValidateState()

		case CommitState:
			fmt.Println("_ OUT _")
			return
		}
	}
}

type currentState struct {
	// parent is the parent hash
	parent types.Hash

	// list of validators
	validators []types.Address

	// the block being processed
	number uint64

	// the selected proposer
	proposer types.Address

	// current view
	view *proto.View

	// list of prepared messages
	prepared map[types.Address]*proto.MessageReq

	// list of commited messages
	committed map[types.Address]*proto.MessageReq
}

func (c *currentState) F() int {
	return int(math.Ceil(float64(len(c.validators))/3)) - 1
}

func (c *currentState) addPrepared(msg *proto.MessageReq) {
	c.prepared[types.StringToAddress(msg.From)] = msg
}

func (c *currentState) numPrepared() int {
	return len(c.prepared)
}

func (c *currentState) numCommited() int {
	return len(c.committed)
}

func (c *currentState) addCommited(msg *proto.MessageReq) {
	c.committed[types.StringToAddress(msg.From)] = msg
}

func (c *currentState) Subject() *proto.Subject {
	return &proto.Subject{
		View: &proto.View{
			Round:    c.view.Round,
			Sequence: c.view.Sequence,
		},
	}
}

func (i *Ibft2) computeCurrentProposal() error {
	if i.state2.parent == types.ZeroHash {
		i.state2.proposer = i.validators[0]
	} else {
		panic("TODO")
	}
	return nil
}

func (i *Ibft2) runAcceptState() {
	// This is the state in which we either propose a block or wait for the pre-prepare message
	fmt.Println("-- i --")
	fmt.Println(i.index)

	number := uint64(1)

	// define the current state
	i.state2 = &currentState{
		parent:     types.Hash{},
		number:     number, // TODO: GetHeader()
		validators: i.validators,
		view: &proto.View{
			Round:    0,
			Sequence: number,
		},
		prepared:  map[types.Address]*proto.MessageReq{},
		committed: map[types.Address]*proto.MessageReq{},
	}

	// start the current state
	if err := i.computeCurrentProposal(); err != nil {
		panic(err)
	}

	if i.state2.proposer == i.validatorKeyAddr {
		// we are the validator
		i.transport.Gossip(i.validators, &proto.MessageReq{
			Message: &proto.MessageReq_Preprepare{
				Preprepare: &proto.Preprepare{
					View: i.state2.view,
				},
			},
		})
	} else {
		// we have to wait for pre-prepare message
		select {
		case msg := <-i.preprepareCh:
			if msg.From != i.state2.proposer.String() {
				panic("bad proposer")
			}

		case <-time.After(5 * time.Second):
			panic("state change")
		}
	}

	// at this point the block is locked?

	// send the prepare message
	i.transport.Gossip(i.validators, &proto.MessageReq{
		Message: &proto.MessageReq_Prepare{
			Prepare: i.state2.Subject(),
		},
	})

	i.setState(ValidateState)
}

func (i *Ibft2) runValidateState() {
	timer := randomTimeout(10 * time.Second)

	hasCommited := false
	sendCommit := func() {
		if !hasCommited {
			// send the prepare message
			i.transport.Gossip(i.validators, &proto.MessageReq{
				Message: &proto.MessageReq_Commit{
					Commit: i.state2.Subject(),
				},
			})
			hasCommited = true
		}
	}

	for i.getState() == ValidateState {
		select {
		case msg := <-i.prepareCh:
			i.state2.addPrepared(msg)

		case msg := <-i.commitCh:
			i.state2.addCommited(msg)

		case <-timer:
			panic("validation timeout")
		}

		// if there are enough prepare messages, send a commit message
		// if there are more than n commit messages, exit and commit block
		// then return to state
		fmt.Println(i.state2.prepared)
		fmt.Println(i.state2.committed)

		if i.state2.numPrepared() > 2*i.state2.F() {
			// we have received enough pre-prepare messages
			sendCommit()
		}

		if i.state2.numCommited() > 2*i.state2.F() {
			// we have received enough commit messages
			sendCommit()

			// try to commit the block
			fmt.Println("__ COMMIT BLOCK __")
			i.setState(CommitState)
		}
	}
}

func (i *Ibft2) getState() IbftState {
	stateAddr := (*uint64)(&i.state)
	return IbftState(atomic.LoadUint64(stateAddr))
}

func (i *Ibft2) setState(s IbftState) {
	i.logger.Debug("state change", "new", s)

	stateAddr := (*uint64)(&i.state)
	atomic.StoreUint64(stateAddr, uint64(s))
}

func randomTimeout(minVal time.Duration) <-chan time.Time {
	if minVal == 0 {
		return nil
	}
	extra := (time.Duration(rand.Int63()) % minVal)
	return time.After(minVal + extra)
}

func (i *Ibft2) StartSeal() {

	// start the transport protocol
	transport, err := i.transportFactory(i)
	if err != nil {
		panic(err)
	}
	i.transport = transport

	go i.run()
}

func (i *Ibft2) run() {
	i.start()
}

func (i *Ibft2) VerifyHeader(parent, header *types.Header) error {
	return nil
}

func (i *Ibft2) Close() error {
	close(i.closeCh)
	return nil
}

// TODO REMOVE
func (i *Ibft2) Prepare(header *types.Header) error {
	return nil
}

// TODO REMOVE
func (i *Ibft2) Seal(block *types.Block, ctx context.Context) (*types.Block, error) {
	return nil, nil
}

type Validators []types.Address

func (i *Ibft2) getNextMessage(stopCh chan struct{}) *proto.MessageReq {
START:
	i.msgQueueLock.Lock()
	if i.msgQueue.Len() != 0 {
		nextMsg := i.msgQueue[0]
		fmt.Println("- next msg -")
		fmt.Println(nextMsg)
	}

	i.msgQueueLock.Unlock()

	// wait until there is a new message or
	// someone closes the stopCh (i.e. timeout for round change)
	select {
	case <-stopCh:
		return nil
	case <-i.updateCh:
		goto START
	}
}

func (i *Ibft2) pushMessage(msg *proto.MessageReq) {
	i.msgQueueLock.Lock()

	// TODO: We can do better
	var view *proto.View
	var msgType uint64

	switch obj := msg.Message.(type) {
	case *proto.MessageReq_Preprepare:
		view = obj.Preprepare.View
		msgType = msgPreprepare

	case *proto.MessageReq_Prepare:
		view = obj.Prepare.View
		msgType = msgPrepare

	case *proto.MessageReq_Commit:
		view = obj.Commit.View
		msgType = msgCommit

	case *proto.MessageReq_RoundChange:
		view = obj.RoundChange.View
		msgType = msgRoundChange
	}

	task := &msgTask{
		sequence: view.Sequence,
		round:    view.Round,
		msg:      msgType,
		obj:      msg,
	}
	heap.Push(&i.msgQueue, task)
	i.msgQueueLock.Unlock()

	select {
	case i.updateCh <- struct{}{}:
	default:
	}
}

/*
* We use a queue to prioritize the consensus messages
 */

const (
	// priority order for the messages
	msgRoundChange = uint64(0)
	msgPreprepare  = uint64(1)
	msgCommit      = uint64(2)
	msgPrepare     = uint64(3)
)

type msgTask struct {
	// priority
	sequence uint64
	round    uint64
	msg      uint64

	obj *proto.MessageReq
}

type msgQueueImpl []*msgTask

// Len returns the length of the queue
func (m msgQueueImpl) Len() int {
	return len(m)
}

// Less compares the priorities of two items at the passed in indexes (A < B)
func (m msgQueueImpl) Less(i, j int) bool {
	ti, tj := m[i], m[j]
	// sort by sequence
	if ti.sequence != tj.sequence {
		return ti.sequence < tj.sequence
	}
	// sort by round
	if ti.round != tj.round {
		return ti.round < tj.round
	}
	// sort by message
	return ti.msg < tj.msg
}

// Swap swaps the places of the items at the passed-in indexes
func (m msgQueueImpl) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// Push adds a new item to the queue
func (m *msgQueueImpl) Push(x interface{}) {
	*m = append(*m, x.(*msgTask))
}

// Pop removes an item from the queue
func (m *msgQueueImpl) Pop() interface{} {
	old := *m
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*m = old[0 : n-1]
	return item
}
