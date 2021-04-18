package ibft

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/consensus/ibft/proto"
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
	sealing uint64

	logger hclog.Logger
	config *consensus.Config
	state  *currentState

	blockchain blockchainInterface
	executor   *state.Executor
	closeCh    chan struct{}

	validatorKey     *ecdsa.PrivateKey
	validatorKeyAddr types.Address

	// snapshot state
	store     *memdb.MemDB
	epochSize uint64

	// queue of messages
	msgQueue *msgQueue
	updateCh chan struct{}

	// sync protocol
	syncer       *protocol.Syncer
	syncNotifyCh chan bool

	network   *network.Server
	transport transport

	operator *operator

	// aux test methods
	forceTimeoutCh bool
}

func Factory(ctx context.Context, config *consensus.Config, txpool *txpool.TxPool, network *network.Server, blockchain *blockchain.Blockchain, executor *state.Executor, srv *grpc.Server, logger hclog.Logger) (consensus.Consensus, error) {
	p := &Ibft2{
		logger:       logger.Named("ibft2"),
		config:       config,
		blockchain:   blockchain,
		executor:     executor,
		closeCh:      make(chan struct{}),
		state:        &currentState{},
		network:      network,
		epochSize:    100000,
		syncNotifyCh: make(chan bool),
		sealing:      0,
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

	// start the transport protocol
	if err := p.setupTransport(); err != nil {
		return nil, err
	}

	// do the start right now but it will only seal blocks if its in sealing mode
	go p.start()

	return p, nil
}

type transport interface {
	Gossip(msg *proto.MessageReq) error
}

var ibftProto = "/ibft/0.1"

type gossipTransport struct {
	topic *network.Topic
}

func (g *gossipTransport) Gossip(msg *proto.MessageReq) error {
	return g.topic.Publish(msg)
}

func (i *Ibft2) setupTransport() error {
	// use a gossip protocol
	topic, err := i.network.NewTopic(ibftProto, &proto.MessageReq{})
	if err != nil {
		return err
	}

	err = topic.Subscribe(func(obj interface{}) {
		msg := obj.(*proto.MessageReq)

		if !i.isSealing() {
			// if we are not sealing we do not care about the messages
			// but we need to subscribe to propagate the messages
			return
		}

		// decode sender
		if err := validateMsg(msg); err != nil {
			i.logger.Error("failed to validate msg", "err", err)
			return
		}

		if msg.From == i.validatorKeyAddr.String() {
			// we are the sender, skip this message since we already
			// relay our own messages internally.
			return
		}
		i.pushMessage(msg)
	})
	if err != nil {
		return err
	}

	i.transport = &gossipTransport{topic: topic}
	return nil
}

func (i *Ibft2) createKey() error {
	// i.msgQueue = msgQueueImpl{}
	i.msgQueue = newMsgQueue()
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

func (i *Ibft2) start() {
	// consensus always starts in SyncState mode in case it needs
	// to sync with other nodes.
	i.setState(SyncState)

	header := i.blockchain.Header()
	i.logger.Debug("current sequence", "sequence", header.Number+1)

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
	if i.state.view != nil {
		i.logger.Debug("cycle", "state", i.getState(), "sequence", i.state.view.Sequence, "round", i.state.view.Round)
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
	}
}

func (i *Ibft2) isValidSnapshot() bool {
	if !i.isSealing() {
		return false
	}

	// check if we are a validator and enabled
	header := i.blockchain.Header()
	snap, err := i.getSnapshot(header.Number)
	if err != nil {
		panic(err)
	}

	if snap.Set.Includes(i.validatorKeyAddr) {
		i.state.view = &proto.View{
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
				i.state.view = &proto.View{
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
			i.setState(AcceptState)
			continue
		}

		// start watch mode
		var isValidator bool
		i.syncer.WatchSyncWithPeer(p, func(b *types.Block) bool {
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
	logger := i.logger.Named("acceptState")
	logger.Info("Accept state", "sequence", i.state.view.Sequence)

	// This is the state in which we either propose a block or wait for the pre-prepare message
	parent := i.blockchain.Header()
	number := parent.Number + 1
	if number != i.state.view.Sequence {
		panic("TODO")
	}
	snap, err := i.getSnapshot(parent.Number)
	if err != nil {
		panic(err)
	}

	i.state.validators = snap.Set

	// reset round messages
	i.state.resetRoundMsgs()

	// select the proposer of the block
	var lastProposer types.Address
	if parent.Number != 0 {
		lastProposer, _ = ecrecoverFromHeader(parent)
	}

	i.state.CalcProposer(lastProposer)

	if i.state.proposer == i.validatorKeyAddr {
		logger.Info("we are the proposer", "block", number)

		if !i.state.locked {
			// since the state is not locked, we need to build a new block
			i.state.block = i.buildBlock(snap, parent)

			// calculate how much time do we have to wait to mine the block
			delay := time.Until(time.Unix(int64(i.state.block.Header.Timestamp), 0))

			select {
			case <-time.After(delay):
			case <-i.closeCh:
				return
			}
		}

		// send the preprepare message as an RLP encoded block
		i.sendPreprepareMsg()

		// send the prepare message since we are ready to move the state
		i.sendPrepareMsg()

		// move to validation state for new prepare messages
		i.setState(ValidateState)
		return
	}

	i.logger.Info("proposer calculated", "proposer", i.state.proposer, "block", number)

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

		if msg.From != i.state.proposer.String() {
			i.logger.Error("msg received from wrong proposer")
			continue
		}

		// retrieve the block proposal
		block := &types.Block{}
		if err := block.UnmarshalRLP(msg.Proposal.Value); err != nil {
			panic(err)
		}
		if i.state.locked {
			// the state is locked, we need to receive the same block
			if block.Hash() == i.state.block.Hash() {
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
				i.state.block = block

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
		i.state.lock()

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

		switch msg.Type {
		case proto.MessageReq_Prepare:
			i.state.addPrepared(msg)

		case proto.MessageReq_Commit:
			i.state.addCommited(msg)

		default:
			panic(fmt.Sprintf("BUG: %s", reflect.TypeOf(msg.Type)))
		}

		if i.state.numPrepared() > i.state.NumValid() {
			// we have received enough pre-prepare messages
			sendCommit()
		}

		if i.state.numCommited() > i.state.NumValid() {
			// we have received enough commit messages
			sendCommit()

			// try to commit the block (TODO: just to get out of the loop)
			i.setState(CommitState)
		}
	}

	if i.getState() == CommitState {
		// at this point either if it works or not we need to unlock
		block := i.state.block
		i.state.unlock()

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

func (i *Ibft2) insertBlock(block *types.Block) error {
	committedSeals := [][]byte{}
	for _, commit := range i.state.committed {
		committedSeals = append(committedSeals, hex.MustDecodeHex(commit.Seal)) // TODO: Validate
	}

	header, err := writeCommittedSeals(block.Header, committedSeals)
	if err != nil {
		return err
	}

	// we need to recompute the hash since we have change extra-data
	block.Header = header
	block.Header.ComputeHash()

	if err := i.blockchain.WriteBlocks([]*types.Block{block}); err != nil {
		return err
	}

	// increase the sequence number and reset the round if any
	i.state.view = &proto.View{
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
	i.state.err = err
	i.setState(RoundChangeState)
}

func (i *Ibft2) runRoundChangeState() {
	sendRoundChange := func(round uint64) {
		i.logger.Debug("local round change", "round", round)
		// set the new round
		i.state.view.Round = round
		// clean the round
		i.state.cleanRound(round)
		// send the round change message
		i.sendRoundChange()
	}
	sendNextRoundChange := func() {
		sendRoundChange(i.state.view.Round + 1)
	}

	checkTimeout := func() {
		// check if there is any peer that is really advanced and we might need to sync with it first
		if i.syncer != nil {
			bestPeer := i.syncer.BestPeer()
			if bestPeer != nil {
				lastProposal := i.blockchain.Header()
				if bestPeer.Number() > lastProposal.Number {
					i.logger.Debug("it has found a better peer to connect", "local", lastProposal.Number, "remote", bestPeer.Number())
					// we need to catch up with the last sequence
					i.setState(SyncState)
					return
				}
			}
		}

		// otherwise, it seems that we are in sync
		// and we should start a new round
		sendNextRoundChange()
	}

	// if the round was triggered due to an error, we send our own
	// next round change
	if err := i.state.getErr(); err != nil {
		i.logger.Debug("round change handle err", "err", err)
		sendNextRoundChange()
	} else {
		// otherwise, it is due to a timeout in any stage
		// First, we try to sync up with any max round already available
		if maxRound, ok := i.state.maxRound(); ok {
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
		msg, ok := i.getNextMessage(timerCh)
		if !ok {
			// closing
			return
		}
		if msg == nil {
			i.logger.Debug("round change timeout")
			checkTimeout()
			continue
		}

		// we only expect RoundChange messages right now
		num := i.state.AddRoundMessage(msg)

		if num == i.state.NumValid() {
			// start a new round inmediatly
			i.state.view.Round = msg.View.Round
			i.setState(AcceptState)
		} else if num == i.state.validators.MinFaultyNodes()+1 {
			// weak certificate, try to catch up if our round number is smaller
			if i.state.view.Round < msg.View.Round {
				// update timer
				timerCh = i.randomTimeout()
				sendRoundChange(msg.View.Round)
			}
		}
	}
}

// --- com wrappers ---

func (i *Ibft2) sendRoundChange() {
	i.gossip(proto.MessageReq_RoundChange)
}

func (i *Ibft2) sendPreprepareMsg() {
	i.gossip(proto.MessageReq_Preprepare)
}

func (i *Ibft2) sendPrepareMsg() {
	i.gossip(proto.MessageReq_Prepare)
}

func (i *Ibft2) sendCommitMsg() {
	i.gossip(proto.MessageReq_Commit)
}

func (i *Ibft2) gossip(typ proto.MessageReq_Type) {
	msg := &proto.MessageReq{
		Type: typ,
	}

	// add View
	msg.View = i.state.view.Copy()

	// if we are sending a preprepare message we need to include the proposed block
	if msg.Type == proto.MessageReq_Preprepare {
		msg.Proposal = &any.Any{
			Value: i.state.block.MarshalRLP(),
		}
	}

	// if the message is commit, we need to add the committed seal
	if msg.Type == proto.MessageReq_Commit {
		seal, err := writeCommittedSeal(i.validatorKey, i.state.block.Header)
		if err != nil {
			i.logger.Error("failed to commit seal", "err", err)
			return
		}
		msg.Seal = hex.EncodeToHex(seal)
	}

	if msg.Type != proto.MessageReq_Preprepare {
		// send a copy to ourselves so that we can process this message as well
		msg2 := msg.Copy()
		msg2.From = i.validatorKeyAddr.String()
		i.pushMessage(msg2)
	}
	if err := signMsg(i.validatorKey, msg); err != nil {
		i.logger.Error("failed to sign message", "err", err)
		return
	}
	if err := i.transport.Gossip(msg); err != nil {
		i.logger.Error("failed to gossip", "err", err)
	}
}

func (i *Ibft2) getState() IbftState {
	return i.state.getState()
}

func (i *Ibft2) isState(s IbftState) bool {
	return i.state.getState() == s
}

func (i *Ibft2) setState(s IbftState) {
	i.logger.Debug("state change", "new", s)
	i.state.setState(s)
}

func (i *Ibft2) forceTimeout() {
	i.forceTimeoutCh = true
}

func (i *Ibft2) randomTimeout() chan struct{} {
	// calculate the timeout duration depending on the current round
	timeout := time.Duration(10000) * time.Millisecond
	round := i.state.view.Round
	if round > 0 {
		timeout += time.Duration(math.Pow(2, float64(round))) * time.Second
	}

	doneCh := make(chan struct{})
	go func() {
		time.Sleep(timeout)
		doneCh <- struct{}{}
	}()
	return doneCh
}

func (i *Ibft2) isSealing() bool {
	return atomic.LoadUint64(&i.sealing) != 0
}

func (i *Ibft2) StartSeal() {
	// start the transport protocol

	// go i.run()
	atomic.StoreUint64(&i.sealing, 1)
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

func (i *Ibft2) getNextMessage(stopCh chan struct{}) (*proto.MessageReq, bool) {
	if stopCh == nil {
		stopCh = make(chan struct{})
	}
	for {
		msg := i.msgQueue.readMessage(i.getState(), i.state.view)
		if msg != nil {
			return msg.obj, true
		}

		if i.forceTimeoutCh {
			i.forceTimeoutCh = false
			return nil, true
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
	task := &msgTask{
		view: msg.View,
		msg:  protoTypeToMsg(msg.Type),
		obj:  msg,
	}
	i.msgQueue.pushMessage(task)

	select {
	case i.updateCh <- struct{}{}:
	default:
	}
}
