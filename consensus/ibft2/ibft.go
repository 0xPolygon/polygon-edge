package ibft2

import (
	"bytes"
	"container/heap"
	"context"
	"crypto/ecdsa"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/protocol"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/txpool"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-memdb"
	"google.golang.org/grpc"
)

type blockchainInterface interface {
	Header() *types.Header
	GetHeaderByNumber(i uint64) (*types.Header, bool)
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

	// sync protocol
	syncer       *protocol.Syncer
	syncNotifyCh chan bool

	sealing bool

	network *network.Server

	transportFactory transportFactory
	transport        transport

	operator        *operator
	maxTimeoutRange time.Duration
}

func Factory(ctx context.Context, config *consensus.Config, txpool *txpool.TxPool, network *network.Server, blockchain *blockchain.Blockchain, executor *state.Executor, srv *grpc.Server, logger hclog.Logger) (consensus.Consensus, error) {
	p := &Ibft2{
		logger:           logger.Named("ibft2"),
		config:           config,
		blockchain:       blockchain,
		executor:         executor,
		closeCh:          make(chan struct{}),
		transportFactory: grpcTransportFactory,
		state2:           &currentState{},
		network:          network,
		maxTimeoutRange:  defaultBlockPeriod,
		epochSize:        100000,
		syncNotifyCh:     make(chan bool),
		sealing:          false,
	}

	p.syncer = protocol.NewSyncer(logger, network, blockchain)
	p.syncer.SyncNotifyCh = p.syncNotifyCh
	p.syncer.Start()

	// register the grpc operator
	p.operator = &operator{i: p}
	proto.RegisterIbftOperatorServer(srv, p.operator)

	if err := p.createKey(); err != nil {
		return nil, err
	}

	p.logger.Info("validator key", "addr", p.validatorKeyAddr.String())

	// start the snapshot
	if err := p.setupSnapshot(); err != nil {
		return nil, err
	}

	transport, err := p.transportFactory(p)
	if err != nil {
		panic(err)
	}
	p.transport = transport

	// always have this active
	go p.readMessages()

	// do the start right now but it will only seal blocks if its in sealing mode
	go p.start()

	return p, nil
}

func (i *Ibft2) createKey() error {
	i.msgQueue = msgQueueImpl{}
	i.closeCh = make(chan struct{})
	i.updateCh = make(chan struct{})

	if i.validatorKey == nil {
		// generate a validator private key
		validatorKey, err := crypto.ReadPrivKey(filepath.Join(i.config.Path, IbftKeyName))
		if err != nil {
			return err
		}
		i.validatorKey = validatorKey
		i.validatorKeyAddr = crypto.PubKeyToAddress(&validatorKey.PublicKey)
	}
	return nil
}

const IbftKeyName = "validator.key"

type IbftState uint32

const (
	AcceptState IbftState = iota
	RoundChangeState
	PreprepareState // TODO: REMOVE

	ValidateState

	// Combine
	PrepareState
	CommitState

	SyncState
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

	case SyncState:
		return "SyncState"
	}
	panic(fmt.Sprintf("BUG: Ibft state not found %d", i))
}

func (i *Ibft2) readMessages() {
	//fmt.Println("XXX")
	//fmt.Println(i.transport)

	ch := i.transport.Listen()

	//fmt.Println("-- ch ..")
	//fmt.Println(ch)

	for {
		msg := <-ch
		i.pushMessage(msg)
	}
}

func (i *Ibft2) waitForGenesis() {
	// TODO
}

func (i *Ibft2) start() {
	// consensus always starts in SyncState mode in case it needs
	// to sync with other nodes.
	i.setState(SyncState)

	header := i.blockchain.Header()
	i.logger.Debug("current sequence", "sequence", header.Number+1)

	// Wait for us to be a validator?

	for {
		select {
		case <-i.closeCh:
			return
		default:
		}

		i.runCycle()
	}
}

func (i *Ibft2) runCycle() {
	if i.state2.view != nil {
		i.logger.Debug("cycle", "state", i.getState(), "sequence", i.state2.view.Sequence, "round", i.state2.view.Round)
	}

	switch i.getState() {
	case AcceptState:
		i.runAcceptState()

	case ValidateState:
		i.runValidateState()

	case RoundChangeState:
		i.runRoundChangeState()

	case SyncState:
		i.runSyncState()

	case CommitState:
		return
	}
}

func (i *Ibft2) isValidSnapshot() bool {

	fmt.Println("-- check if snapshot is valid --")
	fmt.Println(i.isSealing())

	if !i.isSealing() {
		return false
	}

	// check if we are a validator and enabled
	header := i.blockchain.Header()
	snap, err := i.getSnapshot(header.Number)
	if err != nil {
		panic(err)
	}

	fmt.Println(snap.Set)
	if snap.Set.Includes(i.validatorKeyAddr) {
		fmt.Println("INSIDE")

		i.state2.view = &proto.View{
			Sequence: header.Number + 1,
			Round:    0,
		}
		return true
	}
	return false
}

func (i *Ibft2) runSyncState() {
	for i.isState(SyncState) {
		// try to sync with some target peer
		p := i.syncer.BestPeer()
		if p == nil {
			// if we do not have any peers and we have been a validator
			// we can start now. In case we start on another fork this will be
			// reverted later
			if i.isValidSnapshot() {
				// initialize the round and sequence
				header := i.blockchain.Header()
				i.state2.view = &proto.View{
					Round:    0,
					Sequence: header.Number + 1,
				}
				i.setState(AcceptState)
			} else {
				time.Sleep(1 * time.Second)
			}
			continue
		}

		if err := i.syncer.BulkSyncWithPeer(p); err != nil {
			i.logger.Error("failed to bulk sync", "err", err)
			continue
		}

		// if we are a validator we do not even want to wait here
		// we can just move ahead
		if i.isValidSnapshot() {
			fmt.Println("_ IS VALID SNAPSHOT, STOP NOW _")
			i.setState(AcceptState)
			continue
		}

		// start watch mode
		var isValidator bool
		i.syncer.WatchSyncWithPeer(p, func(b *types.Block) bool {
			fmt.Println("_X_X_X_")

			isValidator = i.isValidSnapshot()
			if isValidator {
				return false // stop the handler
			}

			return true
		})

		if isValidator {
			// at this point, we are in sync with the latest chain we know of
			// and we are a validator of that chain so we need to change to AcceptState
			// so that we can start to do some stuff there
			i.setState(AcceptState)
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

	// describes whether there has been an error during the computation
	err error
}

func (c *currentState) NumValid() int {
	/*
		// ceil(2N/3)
		num := float64(len(c.validators))
		return int(math.Ceil(2 * num / 3))
	*/

	// represents the number of required messages
	return 2 * c.validators.MinFaultyNodes()
}

// getErr returns the current error if any and consumes it
func (c *currentState) getErr() error {
	err := c.err
	c.err = nil
	return err
}

func (c *currentState) maxRound() (maxRound uint64, found bool) {
	num := c.validators.MinFaultyNodes() + 1

	for k, round := range c.roundMessages {
		if len(round) < num {
			continue
		}
		if maxRound < k {
			maxRound = k
			found = true
		}
	}
	return
}

func (c *currentState) resetRoundMsgs() {
	c.prepared = map[types.Address]*proto.MessageReq{}
	c.committed = map[types.Address]*proto.MessageReq{}
	c.roundMessages = map[uint64]map[types.Address]*proto.MessageReq{}
}

func (c *currentState) CalcProposer(lastProposer types.Address) {
	c.proposer = c.validators.CalcProposer(c.view.Round, lastProposer)
}

func (c *currentState) lock() {
	c.locked = true
}

func (c *currentState) isLocked() bool {
	return c.locked
}

func (c *currentState) unlock() {
	c.block = nil
	c.locked = false
}

func (c *currentState) cleanRound(round uint64) {
	delete(c.roundMessages, round)
}

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

var defaultBlockPeriod = 10 * time.Second

func (i *Ibft2) buildBlock(snap *Snapshot, parent *types.Header) *types.Block {
	header := &types.Header{
		ParentHash: parent.Hash,
		Number:     parent.Number + 1,
		Miner:      types.Address{},
		Nonce:      types.Nonce{},
		MixHash:    IstanbulDigest,
		Difficulty: parent.Number + 1,   // we need to do this because blockchain needs difficulty to organize blocks and forks
		StateRoot:  types.EmptyRootHash, // this avoids needing state for now
		Sha3Uncles: types.EmptyUncleHash,
	}

	fmt.Println("## ==> BUILD BLOCK (PARENT INFO) <== ##")
	fmt.Println(parent.Hash, parent.Number)

	// try to pick a candidate
	if candidate := i.operator.getNextCandidate(snap); candidate != nil {
		header.Miner = types.StringToAddress(candidate.Address)
		if candidate.Auth {
			header.Nonce = nonceAuthVote
		} else {
			header.Nonce = nonceDropVote
		}
	}

	// set the timestamp
	parentTime := time.Unix(int64(parent.Timestamp), 0)
	headerTime := parentTime.Add(defaultBlockPeriod)

	if headerTime.Before(time.Now()) {
		headerTime = time.Now()
	}
	header.Timestamp = uint64(headerTime.Unix())

	// we need to include in the extra field the current set of validators
	putIbftExtraValidators(header, snap.Set)

	// write the seal of the block
	var err error
	header, err = writeSeal(i.validatorKey, header)
	if err != nil {
		panic(err)
	}

	// TODO: Gather transactions
	block := &types.Block{
		Header: header,
	}

	// compute the hash, this is only a provisional hash since the final one
	// is sealed after all the committed seals
	block.Header.ComputeHash()
	return block
}

func (i *Ibft2) runAcceptState() { // start new round
	i.logger.Info("Accept state", "sequence", i.state2.view.Sequence)

	// This is the state in which we either propose a block or wait for the pre-prepare message
	parent := i.blockchain.Header()
	number := parent.Number + 1
	if number != i.state2.view.Sequence {
		panic("TODO")
	}
	snap, err := i.getSnapshot(parent.Number)
	if err != nil {
		panic(err)
	}

	fmt.Println("-- snapshot --")
	fmt.Println(snap.Set)
	fmt.Println(snap.Votes)

	i.state2.validators = snap.Set

	// reset round messages
	i.state2.resetRoundMsgs()

	// select the proposer of the block
	var lastProposer types.Address
	if parent.Number != 0 {
		lastProposer, _ = ecrecoverFromHeader(parent)
	}

	i.state2.CalcProposer(lastProposer)

	if i.state2.proposer == i.validatorKeyAddr {
		i.logger.Info("we are the proposer", "block", number)

		if !i.state2.locked {
			// since the state is not locked, we need to build a new block
			i.state2.block = i.buildBlock(snap, parent)

			// calculate how much time do we have to wait to mine the block
			delay := time.Until(time.Unix(int64(i.state2.block.Header.Timestamp), 0))

			delay = 2 * time.Second
			fmt.Println("--")
			fmt.Println(delay)

			select {
			case <-time.After(delay):
			case <-i.closeCh:
				return
			}
		}

		fmt.Println("___ HASH WE SEND __")
		fmt.Println(i.state2.block.Hash())

		// send the preprepare message as an RLP encoded block
		i.sendPreprepareMsg()

		// send the prepare message since we are ready to move the state
		i.sendPrepareMsg()

		// move to validation state for new prepare messages
		i.setState(ValidateState)
		return
	}

	i.logger.Info("proposer calculated", "proposer", i.state2.proposer, "block", number)

	// we are NOT a proposer for the block. Then, we have to wait
	// for a pre-prepare message from the proposer

	timerCh := i.randomTimeout()
	for i.getState() == AcceptState {
		msg, ok := i.getNextMessage(timerCh)
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

		fmt.Println("___ HASH WE RECEIVE __")
		fmt.Println(block.Hash())

		if i.state2.locked {
			// the state is locked, we need to receive the same block
			if block.Hash() == i.state2.block.Hash() {
				// fast-track and send a commit message and wait for validations
				i.sendCommitMsg()
				i.setState(ValidateState)
			} else {
				i.handleStateErr(errIncorrectBlockLocked)
			}
		} else {
			// since its a new block, we have to verify it first
			if err := i.verifyHeaderImpl(snap, parent, block.Header); err != nil {
				i.handleStateErr(errBlockVerificationFailed)
			} else {
				i.state2.block = block

				// send prepare message and wait for validations
				i.sendPrepareMsg()
				i.setState(ValidateState)
			}
		}
	}
}

func (i *Ibft2) runValidateState() {
	hasCommited := false
	sendCommit := func() {
		// at this point either we have enough prepare messages
		// or commit messages so we can lock the block
		i.state2.lock()

		if !hasCommited {
			// send the commit message
			i.sendCommitMsg()
			hasCommited = true
		}
	}

	timerCh := i.randomTimeout()
	for i.getState() == ValidateState {
		msg, ok := i.getNextMessage(timerCh)
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

		fmt.Println("- min num valid nodes -")
		fmt.Println(i.state2.NumValid())

		if i.state2.numPrepared() > i.state2.NumValid() {
			// we have received enough pre-prepare messages
			sendCommit()
		}

		if i.state2.numCommited() > i.state2.NumValid() {
			// we have received enough commit messages
			sendCommit()

			// try to commit the block (TODO: just to get out of the loop)
			i.setState(CommitState)
		}
	}

	if i.getState() == CommitState {
		// at this point either if it works or not we need to unlock
		block := i.state2.block
		i.state2.unlock()

		if err := i.insertBlock(block); err != nil {
			// start a new round with the state unlocked since we need to
			// be able to propose/validate a different block
			i.logger.Error("failed to insert block", "err", err)
			i.handleStateErr(errFailedToInsertBlock)
		} else {
			// move ahead to the next block
			i.setState(AcceptState)
		}
	}
}

type sortedBytesSlice [][]byte

func (s sortedBytesSlice) Len() int {
	return len(s)
}

func (s sortedBytesSlice) Less(i, j int) bool {
	return bytes.Compare(s[i], s[j]) < 0
}

func (s sortedBytesSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (i *Ibft2) insertBlock(block *types.Block) error {
	committedSeals := sortedBytesSlice{}
	for _, commit := range i.state2.committed {
		committedSeals = append(committedSeals, hex.MustDecodeHex(commit.Seal)) // TODO: Validate
	}

	// we need to sort the committed seals so that each chain generates
	// the same hash
	sort.Sort(committedSeals)

	header, err := writeCommittedSeals(block.Header, committedSeals)
	if err != nil {
		return err
	}

	// we need to recompute the hash since we have change extra-data
	block.Header = header
	block.Header.ComputeHash()

	fmt.Println("_LAST BLOCK HASH_")
	fmt.Println(block.Header.Hash)

	if err := i.blockchain.WriteBlocks([]*types.Block{block}); err != nil {
		return err
	}

	// increase the sequence number and reset the round if any
	i.state2.view = &proto.View{
		Sequence: header.Number + 1,
		Round:    0,
	}

	// broadcast the new block
	i.syncer.Broadcast(block)
	return nil
}

var (
	errIncorrectBlockLocked    = fmt.Errorf("block locked is incorrect")
	errBlockVerificationFailed = fmt.Errorf("block verification failed")
	errFailedToInsertBlock     = fmt.Errorf("failed to insert block")
)

func (i *Ibft2) handleStateErr(err error) {
	i.state2.err = err
	i.setState(RoundChangeState)
}

func (i *Ibft2) runRoundChangeState() {
	sendRoundChange := func(round uint64) {
		i.logger.Debug("local round change", "round", round)
		// set the new round
		i.state2.view.Round = round
		// clean the round
		i.state2.cleanRound(round)
		// send the round change message
		i.sendRoundChange()
	}
	sendNextRoundChange := func() {
		sendRoundChange(i.state2.view.Round + 1)
	}

	checkTimeout := func() {
		lastProposal := i.blockchain.Header()
		if lastProposal.Number >= i.state2.view.Sequence {
			i.logger.Debug("round change catch up", "current", lastProposal.Number, "sequence", i.state2.view.Sequence)
			// we need to catch up with the last sequence
			i.state2.view = &proto.View{
				Sequence: lastProposal.Number + 1,
			}
			i.setState(AcceptState)
		} else {
			// start a new round
			sendNextRoundChange()
		}
	}

	bestPeer := i.syncer.BestPeer()
	if bestPeer != nil {
		// for now just call the sync protocol
		i.setState(SyncState)
		return
	}

	// if the round was triggered due to an error, we send our own
	// next round change
	if err := i.state2.getErr(); err != nil {
		i.logger.Debug("round change handle err", "err", err)
		sendNextRoundChange()

		// As for now we are not introducing yet these errors here
		panic("STOP")

	} else {
		// otherwise, it is due to a timeout in any stage
		// First, we try to sync up with any max round already available
		if maxRound, ok := i.state2.maxRound(); ok {
			i.logger.Debug("round change set max round", "round", maxRound)
			sendRoundChange(maxRound)
		} else {
			// otherwise, do your best to sync up
			checkTimeout()
		}
	}

	// create a timer for the round change
	timerCh := i.randomTimeout()

	for i.getState() == RoundChangeState {
		raw, ok := i.getNextMessage(timerCh)
		if !ok {
			// closing
			return
		}
		if raw == nil {
			i.logger.Debug("timeout")
			checkTimeout()
			continue
		}

		fmt.Println("-- raw --")
		fmt.Println(raw)

		// we only expect RoundChange messages right now
		view := raw.View()
		num := i.state2.AddRoundMessage(raw)

		if num == i.state2.NumValid() {
			// start a new round inmediatly
			i.state2.view.Round = view.Round
			i.setState(AcceptState)
		} else if num == i.state2.validators.MinFaultyNodes()+1 {
			// weak certificate, try to catch up if our round number is smaller
			if i.state2.view.Round < view.Round {
				// update timer
				timerCh = i.randomTimeout()
				sendRoundChange(view.Round)
			}
		}
	}
}

// --- com wrappers ---

func (i *Ibft2) sendRoundChange() {
	i.gossip(i.state2.validators, &proto.MessageReq{
		Message: &proto.MessageReq_RoundChange{
			RoundChange: i.state2.Subject(),
		},
	})
}

func (i *Ibft2) sendPreprepareMsg() {
	blockRlp := i.state2.block.MarshalRLP()

	i.gossip(i.state2.validators, &proto.MessageReq{
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

func (i *Ibft2) sendPrepareMsg() {
	i.gossip(i.state2.validators, &proto.MessageReq{
		Message: &proto.MessageReq_Prepare{
			Prepare: i.state2.Subject(),
		},
	})
}

func (i *Ibft2) sendCommitMsg() {
	i.gossip(i.state2.validators, &proto.MessageReq{
		Message: &proto.MessageReq_Commit{
			Commit: i.state2.Subject(),
		},
	})
}

func (i *Ibft2) gossip(target []types.Address, msg *proto.MessageReq) {
	// Add the sender of the message
	msg.From = i.validatorKeyAddr.String()

	// if the message is commit, we need to add the committed seal
	if _, ok := msg.Message.(*proto.MessageReq_Commit); ok {
		seal, err := writeCommittedSeal(i.validatorKey, i.state2.block.Header)
		if err != nil {
			panic(err)
		}
		msg.Seal = hex.EncodeToHex(seal)
	}

	if _, ok := msg.Message.(*proto.MessageReq_Preprepare); !ok {
		// copy msg
		msg2 := msg.Copy()
		i.pushMessage(msg2)
	}

	// sign the message
	signMsg, err := msg.PayloadNoSig()
	if err != nil {
		panic(err)
	}
	sig, err := crypto.Sign(i.validatorKey, crypto.Keccak256(signMsg))
	if err != nil {
		panic(err)
	}
	msg.Signature = hex.EncodeToHex(sig)

	i.transport.Gossip(target, msg)
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

func (i *Ibft2) randomTimeout() chan struct{} {
	doneCh := make(chan struct{})
	go func() {
		<-time.After(i.maxTimeoutRange)
		doneCh <- struct{}{}
	}()
	return doneCh
}

func (i *Ibft2) isSealing() bool {
	return i.sealing
}

func (i *Ibft2) StartSeal() {
	// start the transport protocol

	// go i.run()
	i.sealing = true
}

func (i *Ibft2) run() {

}

func (i *Ibft2) verifyHeaderImpl(snap *Snapshot, parent, header *types.Header) error {
	// ensure the extra data is correctly formatted
	if _, err := getIbftExtra(header); err != nil {
		return err
	}
	if header.Nonce != nonceDropVote && header.Nonce != nonceAuthVote {
		return fmt.Errorf("invalid nonce")
	}
	if header.MixHash != IstanbulDigest {
		return fmt.Errorf("invalid mixhash")
	}
	if header.Sha3Uncles != types.EmptyUncleHash {
		return fmt.Errorf("invalid sha3 uncles")
	}
	// difficulty has to match number
	if header.Difficulty != header.Number {
		return fmt.Errorf("wrong difficulty")
	}

	// verify the sealer
	if err := verifySigner(snap, header); err != nil {
		return err
	}
	return nil
}

func (i *Ibft2) VerifyHeader(parent, header *types.Header) error {
	// fmt.Println("__ VERIFY HEADER __")

	snap, err := i.getSnapshot(parent.Number)
	if err != nil {
		return err
	}
	// verify all the header fields + seal
	if err := i.verifyHeaderImpl(snap, parent, header); err != nil {
		return err
	}
	// verify the commited seals
	if err := verifyCommitedFields(snap, header); err != nil {
		return err
	}

	// process the new block in order to update the snapshot
	if err := i.processHeaders([]*types.Header{header}); err != nil {
		return err
	}
	return nil
}

func (i *Ibft2) Close() error {
	close(i.closeCh)
	return nil
}

type Validators []types.Address

func (i *Ibft2) popMessage() *proto.MessageReq {
	i.msgQueueLock.Lock()
	defer i.msgQueueLock.Unlock()

START:
	if i.msgQueue.Len() == 0 {
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

	// we only accept???
	if i.isState(RoundChangeState) {
		if nextMsg.msg != msgRoundChange {
			heap.Pop(&i.msgQueue)
			goto START
		}
	}

	fmt.Println("-- msg --")
	fmt.Println("====>", nextMsg.msg.String(), nextMsg.view.Sequence, nextMsg.view.Round)

	// TODO: If message is roundChange do some check first
	// TODO: If we are in round change we only accept stateChange messages

	if cmpView(nextMsg.view, i.state2.view) > 0 {
		// nextMsg is higher in either round (state change) or sequence
		// (future proposal). We need to wait for a newer message
		return nil
	}

	if cmpView(nextMsg.view, i.state2.view) < 0 {
		fmt.Printf("old value %d %d\n", nextMsg.view.Sequence, nextMsg.view.Round)
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
		if msg != nil {
			i.logger.Debug("process message")
			// fmt.Println(msg)
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
	var msgType MsgType

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
	heap.Push(&i.msgQueue, task)
	i.msgQueueLock.Unlock()

	select {
	case i.updateCh <- struct{}{}:
	default:
	}
}

type MsgType uint64

const (
	// priority order for the messages
	msgRoundChange MsgType = 0
	msgPreprepare  MsgType = 1
	msgCommit      MsgType = 2
	msgPrepare     MsgType = 3
)

func (m MsgType) String() string {
	switch m {
	case msgRoundChange:
		return "RoundChange"
	case msgPrepare:
		return "Prepare"
	case msgPreprepare:
		return "Preprepare"
	case msgCommit:
		return "Commit"
	default:
		panic("BUG")
	}
}

type msgTask struct {
	// priority
	view *proto.View
	msg  MsgType

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
