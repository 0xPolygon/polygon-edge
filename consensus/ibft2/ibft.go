package ibft2

import (
	"container/heap"
	"context"
	"crypto/ecdsa"
	"fmt"
	"path/filepath"
	"reflect"
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
	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-memdb"
)

type blockchainInterface interface {
	Header() *types.Header
	WriteBlocks(blocks []*types.Block) error
}

type Ibft2 struct {
	logger hclog.Logger
	config *consensus.Config

	state  uint64
	state2 *currentState

	blockchain blockchainInterface
	executor   *state.Executor
	closeCh    chan struct{}

	validatorKey     *ecdsa.PrivateKey
	validatorKeyAddr types.Address

	// snapshot state
	store     *memdb.MemDB
	epochSize uint64

	// We use a message queue to process messages in order
	msgQueueLock sync.Mutex
	msgQueue     msgQueueImpl
	updateCh     chan struct{}

	// TODOOOO: We take this info from the blockchain
	// validators []types.Address

	network *network.Server

	transportFactory transportFactory
	transport        transport

	maxTimeoutRange time.Duration
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
		//preprepareCh:     make(chan *proto.MessageReq, 10),
		//commitCh:         make(chan *proto.MessageReq, 10),
		//prepareCh:        make(chan *proto.MessageReq, 10),
	}
	p.createKey()
	return p, nil
}

func (i *Ibft2) setup() error {
	db, err := memdb.NewMemDB(schema)
	if err != nil {
		return err
	}
	i.store = db
	return nil
}

func (i *Ibft2) createKey() error {
	i.msgQueue = msgQueueImpl{}
	i.updateCh = make(chan struct{})

	if i.validatorKey == nil {
		// generate a validator private key
		validatorKey, err := crypto.ReadPrivKey(filepath.Join(i.config.Path, "validator.key"))
		if err != nil {
			return err
		}
		i.validatorKey = validatorKey
		i.validatorKeyAddr = crypto.PubKeyToAddress(&validatorKey.PublicKey)
	}
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
		fmt.Println("-- read msg --")
		fmt.Println(msg)
		i.pushMessage(msg)
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

		case RoundChangeState:
			i.runRoundChangeState()

		case CommitState:
			fmt.Println("_ OUT _")
			return
		}
	}
}

type currentState struct {
	// the snapshot being currently used
	validators ValidatorSet

	// the proposed block
	block *types.Block

	// the selected proposer
	proposer types.Address

	// current view
	view *proto.View

	// list of prepared messages
	prepared map[types.Address]*proto.MessageReq

	// list of commited messages
	committed map[types.Address]*proto.MessageReq

	// list of round change messages
	roundMessages map[uint64]map[types.Address]*proto.MessageReq

	// locked signals whether the proposal is locked
	locked bool
}

func (c *currentState) CalcProposer() {
	c.proposer = c.validators.CalcProposer(c.view.Round, types.ZeroAddress)
}

func (c *currentState) lock() {
	c.locked = true
}

func (c *currentState) isLocked() bool {
	return c.locked
}

func (c *currentState) resetProposal() {
	c.block = nil
	// messages...
}

func (c *currentState) cleanRound(round uint64) {
	panic("TODO")
}

// TODO: Use only one addMessage and use messagetype

func (c *currentState) numRounds(round uint64) int {
	if c.roundMessages == nil {
		return 0
	}
	obj, ok := c.roundMessages[round]
	if !ok {
		return 0
	}
	return len(obj)
}

func (c *currentState) AddRoundMessage(msg *proto.MessageReq) int {
	if c.roundMessages == nil {
		c.roundMessages = map[uint64]map[types.Address]*proto.MessageReq{}
	}
	// validate round change message
	obj, ok := msg.Message.(*proto.MessageReq_RoundChange)
	if !ok {
		panic("BUG: Msg different from state change found")
	}
	addr := types.StringToAddress(msg.From)
	view := obj.RoundChange.View
	if _, ok := c.roundMessages[view.Round]; !ok {
		c.roundMessages[view.Round] = map[types.Address]*proto.MessageReq{}
	}
	if c.validators.Includes(addr) {
		c.roundMessages[view.Round][addr] = msg
	}
	return len(c.roundMessages[view.Round])
}

func (c *currentState) MinFaultyNodes() int {
	return c.validators.MinFaultyNodes()
}

func (c *currentState) addPrepared(msg *proto.MessageReq) {
	if c.prepared == nil {
		c.prepared = map[types.Address]*proto.MessageReq{}
	}
	// validate prepared message
	if _, ok := msg.Message.(*proto.MessageReq_Prepare); !ok {
		return
	}
	// validate address
	addr := types.StringToAddress(msg.From)
	if c.validators.Includes(addr) {
		c.prepared[addr] = msg
	}
}

func (c *currentState) addCommited(msg *proto.MessageReq) {
	if c.committed == nil {
		c.committed = map[types.Address]*proto.MessageReq{}
	}
	// validate prepared message
	if _, ok := msg.Message.(*proto.MessageReq_Commit); !ok {
		return
	}
	// validate address
	addr := types.StringToAddress(msg.From)
	if c.validators.Includes(addr) {
		c.committed[addr] = msg
	}
}

func (c *currentState) numPrepared() int {
	return len(c.prepared)
}

func (c *currentState) numCommited() int {
	return len(c.committed)
}

func (c *currentState) Subject() *proto.Subject {
	return &proto.Subject{
		View: &proto.View{
			Round:    c.view.Round,
			Sequence: c.view.Sequence,
		},
	}
}

func (i *Ibft2) buildBlock(parent *types.Header) *types.Block {
	// TODO: Gather transactions
	block := &types.Block{
		Header: &types.Header{
			ParentHash: parent.Hash,
			Number:     parent.Number + 1,
			Miner:      i.validatorKeyAddr,
		},
	}

	// compute the hash
	block.Header.ComputeHash()
	return block
}

func (i *Ibft2) runAcceptState() { // start new round
	i.logger.Info("Accept state")

	// This is the state in which we either propose a block or wait for the pre-prepare message
	parent := i.blockchain.Header()
	number := parent.Number + 1

	// TODO: get validators
	snap, err := i.getSnapshot(parent.Number)
	if err != nil {
		panic(err)
	}

	// define the current state
	i.state2 = &currentState{
		validators: snap.Set,
		view: &proto.View{
			Round:    0,
			Sequence: number,
		},
	}

	// select the proposer of the block
	i.state2.CalcProposer()
	i.logger.Info("proposer calculated", "proposer", i.state2.proposer, "block", number)

	// TODO: Reset roundchange state

	if i.state2.proposer == i.validatorKeyAddr {
		if !i.state2.locked {
			// since the state is not locked, we need to build a new block
			i.state2.block = i.buildBlock(parent)
		}

		// send the preprepare message as an RLP encoded block
		i.sendPreprepare()

		// send the prepare message since we are ready to move the state
		i.sendPrepare()

		// move to validation state for new prepare messages
		i.setState(ValidateState)
		return
	}

	// we are NOT a proposer for the block. Then, we have to wait
	// for a pre-prepare message from the proposer

	closeCh := make(chan struct{})
	go func() {
		<-time.After(1 * time.Second)
		close(closeCh)
	}()

	for i.getState() == AcceptState {
		msg, ok := i.getNextMessage(closeCh)
		if !ok {
			return
		}
		if msg == nil {
			i.setState(RoundChangeState)
			continue
		}

		preprepare, _ := msg.Message.(*proto.MessageReq_Preprepare)
		if msg.From != i.state2.proposer.String() {
			i.logger.Error("msg received from wrong proposer")
			continue
		}

		// retrieve the block proposal
		block := &types.Block{}
		if err := block.UnmarshalRLP(preprepare.Preprepare.Proposal.Block.Value); err != nil {
			panic(err)
		}

		if i.state2.locked {
			// the state is locked, we need to receive the same block
			if block.Hash() == i.state2.block.Hash() {
				// fast-track and send a commit message and wait for validations
				i.sendCommit()
				i.setState(ValidateState)
			} else {
				i.sendNextRoundChange()
			}
		} else {
			i.state2.block = block

			// send prepare message and wait for validations
			i.sendPrepare()
			i.setState(ValidateState)
		}
	}
}

func (i *Ibft2) runValidateState() {
	timer := i.randomTimeout(10 * time.Second)

	hasCommited := false
	sendCommit := func() {
		// at this point either we have enough prepare messages
		// or commit messages so we can lock the block
		i.state2.lock()

		if !hasCommited {
			// send the commit message
			i.sendCommit()
			hasCommited = true
		}
	}

	stopCh := make(chan struct{})
	go func() {
		<-timer
		close(stopCh)
	}()

	for i.getState() == ValidateState {
		msg, ok := i.getNextMessage(stopCh)
		if !ok {
			// closing
			return
		}
		if msg == nil {
			i.setState(RoundChangeState)
			continue
		}

		switch msg.Message.(type) {
		case *proto.MessageReq_Prepare:
			i.state2.addPrepared(msg)

		case *proto.MessageReq_Commit:
			i.state2.addCommited(msg)

		case *proto.MessageReq_Preprepare:
			// TODO: We might receive this here if we are the proposer since
			// we send all the messages to ourselves too
			continue

		default:
			panic(fmt.Sprintf("BUG: %s", reflect.TypeOf(msg.Message)))
		}

		fmt.Println("-- min faulty nodes --")
		fmt.Println(i.state2.MinFaultyNodes())
		fmt.Println(i.state2.numCommited())

		if i.state2.numPrepared() > 2*i.state2.MinFaultyNodes() {
			// we have received enough pre-prepare messages
			sendCommit()
		}

		if i.state2.numCommited() > 2*i.state2.MinFaultyNodes() {
			// we have received enough commit messages
			sendCommit()

			// try to commit the block (TODO: just to get out of the loop)
			i.setState(CommitState)
		}
	}

	if i.getState() == CommitState {
		// write the block!!
		/**
		fmt.Println("-- write block --")
		if err := i.blockchain.WriteBlocks([]*types.Block{i.state2.block}); err != nil {
			panic(err)
		}
		fmt.Println("-- correct --")
		*/
		i.setState(AcceptState)
	}
}

func (i *Ibft2) runRoundChangeState() {
	/*
		if !c.waitingForRoundChange {
			maxRound := c.roundChangeSet.MaxRound(c.valSet.F() + 1)
			if maxRound != nil && maxRound.Cmp(c.current.Round()) > 0 {
				c.sendRoundChange(maxRound)
				return
			}
		}

		lastProposal, _ := c.backend.LastProposal()
		if lastProposal != nil && new(big.Int).SetUint64(lastProposal.Number()).Cmp(c.current.Sequence()) >= 0 {
			c.logger.Trace("round change timeout, catch up latest sequence", "number", lastProposal.Number())
			c.startNewRound(types.Big0)
		} else {
			c.sendNextRoundChange()
		}
	*/

	// set timer
	// timerCh := randomTimeout(10 * time.Second)

	// we are not waiting for any round change yet
	// check who is the max round and set a round change

	for i.getState() == RoundChangeState {
		raw, ok := i.getNextMessage(nil)
		if !ok {
			// closing
		}
		if raw == nil {
			// catch up with another round
		}

		// we only expect RoundChange messages right now
		view := raw.View()
		num := i.state2.AddRoundMessage(raw)

		if num == i.state2.MinFaultyNodes()+1 {
			// weak certificate, try to catch up if our round number is smaller
			if i.state2.view.Round < view.Round {
				// update timer
				i.sendRoundChange(view.Round)
			}
		} else if num == 2*i.state2.MinFaultyNodes()+1 {
			// start a new round inmediatly
			// change the view
			// change the state
		}
	}
}

func (i *Ibft2) sendNextRoundChange() {
	i.sendRoundChange(i.state2.view.Round + 1)
	i.setState(RoundChangeState)
}

func (i *Ibft2) sendRoundChange(round uint64) {
	// update the current state

	// new state2...
	if i.state2.isLocked() {
		// the proposal is locked, we need to use the same one
		// newstate2.proposal ..
	}

	// clean the round
	i.state2.cleanRound(round)

	// Gossip
	i.transport.Gossip(i.state2.validators, &proto.MessageReq{
		Message: &proto.MessageReq_RoundChange{},
	})
}

// --- com wrappers ---

func (i *Ibft2) sendPreprepare() {
	blockRlp := i.state2.block.MarshalRLP()

	i.transport.Gossip(i.state2.validators, &proto.MessageReq{
		Message: &proto.MessageReq_Preprepare{
			Preprepare: &proto.Preprepare{
				View: i.state2.view,
				Proposal: &proto.Proposal{
					Block: &any.Any{
						Value: blockRlp,
					},
				},
			},
		},
	})
}

func (i *Ibft2) sendPrepare() {
	i.transport.Gossip(i.state2.validators, &proto.MessageReq{
		Message: &proto.MessageReq_Prepare{
			Prepare: i.state2.Subject(),
		},
	})
}

func (i *Ibft2) sendCommit() {
	i.transport.Gossip(i.state2.validators, &proto.MessageReq{
		Message: &proto.MessageReq_Commit{
			Commit: i.state2.Subject(),
		},
	})
}

func (i *Ibft2) getState() IbftState {
	stateAddr := (*uint64)(&i.state)
	return IbftState(atomic.LoadUint64(stateAddr))
}

func (i *Ibft2) isState(s IbftState) bool {
	return i.getState() == s
}

func (i *Ibft2) setState(s IbftState) {
	i.logger.Debug("state change", "new", s)

	stateAddr := (*uint64)(&i.state)
	atomic.StoreUint64(stateAddr, uint64(s))
}

func (i *Ibft2) randomTimeout(minVal time.Duration) <-chan time.Time {
	return time.After(i.maxTimeoutRange)
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

func (i *Ibft2) popMessage() *proto.MessageReq {
	i.msgQueueLock.Lock()
	defer i.msgQueueLock.Unlock()

START:
	if i.msgQueue.Len() == 0 {
		fmt.Println("aaaaaaaa")
		return nil
	}
	nextMsg := i.msgQueue[0]

	// If we are in preprepare we only accept prepreare messages
	if i.isState(AcceptState) {
		if nextMsg.msg != msgPreprepare {
			heap.Pop(&i.msgQueue)
			goto START
		}
	}

	// TODO: If message is roundChange do some check first
	// TODO: If we are in round change we only accept stateChange messages

	if cmpView(nextMsg.view, i.state2.view) > 0 {
		// nextMsg is higher in either round (state change) or sequence
		// (future proposal). We need to wait for a newer message
		return nil
	}

	if cmpView(nextMsg.view, i.state2.view) < 0 {
		// nextMsg is old. Pop the value from the queue and try again
		heap.Pop(&i.msgQueue)
		goto START
	}

	// TODO: If in state accepted state we only accept pre-prepare messages
	heap.Pop(&i.msgQueue)
	return nextMsg.obj
}

func (i *Ibft2) getNextMessage(stopCh chan struct{}) (*proto.MessageReq, bool) {
	if stopCh == nil {
		stopCh = make(chan struct{})
	}
	for {
		msg := i.popMessage()
		fmt.Println("--sssss")
		fmt.Println(msg)
		if msg != nil {
			return msg, true
		}

		// wait until there is a new message or
		// someone closes the stopCh (i.e. timeout for round change)
		select {
		case <-stopCh:
			return nil, true
		case <-i.closeCh:
			return nil, false
		case <-i.updateCh:
		}
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
		view: view,
		msg:  msgType,
		obj:  msg,
	}
	fmt.Println("- -push -")
	heap.Push(&i.msgQueue, task)
	fmt.Println(i.msgQueue)
	i.msgQueueLock.Unlock()

	select {
	case i.updateCh <- struct{}{}:
	default:
	}
}

const (
	// priority order for the messages
	msgRoundChange = uint64(0)
	msgPreprepare  = uint64(1)
	msgCommit      = uint64(2)
	msgPrepare     = uint64(3)
)

type msgTask struct {
	// priority
	view *proto.View
	msg  uint64

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
	if ti.view.Sequence != tj.view.Sequence {
		return ti.view.Sequence < tj.view.Sequence
	}
	// sort by round
	if ti.view.Round != tj.view.Round {
		return ti.view.Round < tj.view.Round
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

func cmpView(v, y *proto.View) int {
	if v.Sequence != y.Sequence {
		if v.Sequence < y.Sequence {
			return -1
		} else {
			return 1
		}
	}
	if v.Round != y.Round {
		if v.Round < y.Round {
			return -1
		} else {
			return 1
		}
	}
	return 0
}
