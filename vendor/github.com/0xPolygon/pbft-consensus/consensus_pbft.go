package pbft

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/0xPolygon/pbft-consensus/stats"
)

type NodeID string

type State uint32

// Define the states in PBFT
const (
	AcceptState State = iota
	RoundChangeState
	ValidateState
	CommitState
	SyncState
	DoneState
)

// String returns the string representation of the passed in state
func (i State) String() string {
	switch i {
	case AcceptState:
		return "AcceptState"
	case RoundChangeState:
		return "RoundChangeState"
	case ValidateState:
		return "ValidateState"
	case CommitState:
		return "CommitState"
	case SyncState:
		return "SyncState"
	case DoneState:
		return "DoneState"
	}
	panic(fmt.Sprintf("BUG: Pbft state not found %d", i))
}

type CommittedSeal struct {
	// Signature value
	Signature []byte

	// Node that signed
	NodeID NodeID
}

// SealedProposal represents the sealed proposal model
type SealedProposal struct {
	Proposal       *Proposal
	CommittedSeals []CommittedSeal
	Proposer       NodeID
	Number         uint64
}

// RoundInfo is the information about the round
type RoundInfo struct {
	IsProposer   bool
	Proposer     NodeID
	Locked       bool
	CurrentRound uint64
}

// Pbft represents the PBFT consensus mechanism object
type Pbft struct {
	// Output logger
	logger Logger

	// Config is the configuration of the consensus
	config *Config

	// inter is the interface with the runtime
	backend Backend

	// state is the reference to the current state machine
	state *state

	// validator is the signing key for this instance
	validator SignKey

	// ctx is the current execution context for an pbft round
	ctx context.Context

	// msgQueue is a queue that stores all the incomming gossip messages
	msgQueue *msgQueue

	// updateCh is a channel used to notify when a new gossip message arrives
	updateCh chan struct{}

	// Transport is the interface for the gossip transport
	transport Transport

	// tracer is a reference to the OpenTelemetry tracer
	tracer trace.Tracer

	// roundTimeout calculates timeout for a specific round
	roundTimeout RoundTimeout

	// notifier is a reference to the struct which encapsulates handling messages and timeouts
	notifier StateNotifier

	stats *stats.Stats
}

// New creates a new instance of the PBFT state machine
func New(validator SignKey, transport Transport, opts ...ConfigOption) *Pbft {
	config := DefaultConfig()
	config.ApplyOps(opts...)

	p := &Pbft{
		validator:    validator,
		state:        newState(),
		transport:    transport,
		msgQueue:     newMsgQueue(),
		updateCh:     make(chan struct{}, 1), //hack. There is a bug when you have several messages pushed on the same time.
		config:       config,
		logger:       config.Logger,
		tracer:       config.Tracer,
		roundTimeout: config.RoundTimeout,
		notifier:     config.Notifier,
		stats:        stats.NewStats(),
	}

	p.logger.Printf("[INFO] validator key: addr=%s\n", p.validator.NodeID())
	return p
}

func (p *Pbft) SetBackend(backend Backend) error {
	p.backend = backend

	// set the next current sequence for this iteration
	p.setSequence(p.backend.Height())

	// set the current set of validators
	p.state.validators = p.backend.ValidatorSet()

	return nil
}

// Run starts the PBFT consensus state machine
func (p *Pbft) Run(ctx context.Context) {
	p.SetInitialState(ctx)

	// start the trace span
	spanCtx, span := p.tracer.Start(context.Background(), fmt.Sprintf("Sequence-%d", p.state.view.Sequence))
	defer span.End()

	// loop until we reach the a finish state
	for p.getState() != DoneState && p.getState() != SyncState {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Start the state machine loop
		p.runCycle(spanCtx)

		// emit stats when the round is ended
		p.emitStats()
	}
}

func (p *Pbft) SetInitialState(ctx context.Context) {
	p.ctx = ctx

	// the iteration always starts with the AcceptState.
	// AcceptState stages will reset the rest of the message queues.
	p.setState(AcceptState)
}

func (p *Pbft) emitStats() {
	if p.config.StatsCallback != nil {
		p.config.StatsCallback(p.stats.Snapshot())
		// once emitted, reset the Stats
		p.stats.Reset()
	}
}

func (p *Pbft) RunCycle(ctx context.Context) {
	p.runCycle(ctx)
}

// runCycle represents the PBFT state machine loop
func (p *Pbft) runCycle(ctx context.Context) {
	startTime := time.Now()
	state := p.getState()
	defer p.stats.StateDuration(state.String(), startTime)

	// Log to the console
	if p.state.view != nil {
		p.logger.Printf("[DEBUG] cycle: state=%s, sequence=%d, round=%d", p.getState(), p.state.view.Sequence, p.state.GetCurrentRound())
	}
	// Based on the current state, execute the corresponding section
	switch state {
	case AcceptState:
		p.runAcceptState(ctx)

	case ValidateState:
		p.runValidateState(ctx)

	case RoundChangeState:
		p.runRoundChangeState(ctx)

	case CommitState:
		p.runCommitState(ctx)

	case DoneState:
		panic("BUG: We cannot iterate on DoneState")
	}
}

func (p *Pbft) setSequence(sequence uint64) {
	p.state.view = &View{
		Sequence: sequence,
	}
	p.setRound(0)
	p.state.unlock()
}

func (p *Pbft) setRound(round uint64) {
	p.state.SetCurrentRound(round)

	// reset current timeout and start a new one
	p.state.timeoutChan = p.roundTimeout(round)
}

// runAcceptState runs the Accept state loop
//
// The Accept state always checks the snapshot, and the validator set. If the current node is not in the validators set,
// it moves back to the Sync state. On the other hand, if the node is a validator, it calculates the proposer.
// If it turns out that the current node is the proposer, it builds a proposal, and sends preprepare and then prepare messages.
func (p *Pbft) runAcceptState(ctx context.Context) { // start new round
	_, span := p.tracer.Start(ctx, "AcceptState")
	defer span.End()

	p.stats.SetView(p.state.view.Sequence, p.state.view.Round)

	if !p.state.validators.Includes(p.validator.NodeID()) {
		// we are not a validator anymore, move back to sync state
		p.logger.Print("[INFO] we are not a validator anymore")
		p.setState(SyncState)
		return
	}

	// reset round messages
	p.state.resetRoundMsgs()
	p.state.CalcProposer()

	isProposer := p.state.proposer == p.validator.NodeID()
	p.backend.Init(&RoundInfo{
		Proposer:     p.state.proposer,
		IsProposer:   isProposer,
		Locked:       p.state.IsLocked(),
		CurrentRound: p.state.GetCurrentRound(),
	})

	// log the current state of this span
	span.SetAttributes(
		attribute.Bool("isproposer", isProposer),
		attribute.Bool("locked", p.state.IsLocked()),
		attribute.String("proposer", string(p.state.proposer)),
	)

	p.logger.Printf("[INFO] accept state: sequence=%d, round=%d, isproposer=%t, locked=%t, proposer=%s", p.state.view.Sequence, p.state.view.Round, isProposer, p.state.IsLocked(), string(p.state.proposer))
	if p.state.IsLocked() {
		p.logger.Printf("[DEBUG] locked proposal: hash=0x%x", p.state.proposal.Hash)
	}

	var err error

	if isProposer {
		p.logger.Printf("[INFO] we are the proposer")

		if !p.state.IsLocked() {
			// since the state is not locked, we need to build a new proposal
			p.state.proposal, err = p.backend.BuildProposal()
			if err != nil {
				p.logger.Printf("[ERROR] failed to build proposal: %v", err)
				p.setState(RoundChangeState)
				return
			}

			// calculate how much time do we have to wait to gossip the proposal
			delay := time.Until(p.state.proposal.Time)

			p.logger.Printf("[INFO] proposal generated: hash=0x%x, delay=%s", p.state.proposal.Hash, delay)

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
		}

		// send the preprepare message
		p.sendPreprepareMsg()

		// send the prepare message since we are ready to move the state
		p.sendPrepareMsg()

		// move to validation state for new prepare messages
		p.setState(ValidateState)
		return
	}

	// we are NOT a proposer for this height/round. Then, we have to wait
	// for a pre-prepare message from the proposer

	// We only need to wait here for one type of message, the Prepare message from the proposer.
	// However, since we can receive bad Prepare messages we have to wait (or timeout) until
	// we get the message from the correct proposer.
	for p.getState() == AcceptState {
		msg, ok := p.getNextMessage(span)
		if !ok {
			return
		}
		if msg == nil {
			p.setState(RoundChangeState)
			continue
		}
		// TODO: Validate that the fields required for Preprepare are set (Proposal and Hash)
		if msg.From != p.state.proposer {
			p.logger.Printf("[ERROR] msg received from wrong proposer: expected=%s, found=%s", p.state.proposer, msg.From)
			continue
		}

		// retrieve the proposal, the backend MUST validate that the hash belongs to the proposal
		proposal := &Proposal{
			Data: msg.Proposal,
			Hash: msg.Hash,
		}
		if p.state.IsLocked() && !p.state.proposal.Equal(proposal) {
			p.handleStateErr(errIncorrectLockedProposal)
			return
		}

		if err := p.backend.Validate(proposal); err != nil {
			p.logger.Printf("[ERROR] failed to validate proposal. Error message: %v", err)
			p.setState(RoundChangeState)
			return
		}

		if p.state.IsLocked() {
			// fast-track and send a commit message and wait for validations
			p.sendCommitMsg()
			p.setState(ValidateState)
		} else {
			p.state.proposal = proposal
			p.sendPrepareMsg()
			p.setState(ValidateState)
		}
	}
}

// runValidateState implements the Validate state loop.
//
// The Validate state is rather simple - all nodes do in this state is read messages and add them to their local snapshot state
func (p *Pbft) runValidateState(ctx context.Context) { // start new round
	_, span := p.tracer.Start(ctx, "ValidateState")
	// set the attributes of this span once runValidateState is done
	defer func() {
		p.setStateSpanAttributes(span)
		span.End()
	}()

	hasCommitted := false
	sendCommit := func(span trace.Span) {
		// at this point either we have enough prepare messages
		// or commit messages so we can lock the proposal
		p.state.lock()

		if !hasCommitted {
			// send the commit message
			p.sendCommitMsg()
			hasCommitted = true

			span.AddEvent("Commit")
		}
	}

	for p.getState() == ValidateState {
		msg, ok := p.getNextMessage(span)
		if !ok {
			// closing
			return
		}
		if msg == nil {
			// timeout
			p.setState(RoundChangeState)
			return
		}

		// the message must have our local hash
		if !bytes.Equal(msg.Hash, p.state.proposal.Hash) {
			p.logger.Printf(fmt.Sprintf("[WARN] incorrect hash in %s message", msg.Type.String()))
			continue
		}

		switch msg.Type {
		case MessageReq_Prepare:
			p.state.addPrepared(msg)

		case MessageReq_Commit:
			if err := p.backend.ValidateCommit(msg.From, msg.Seal); err != nil {
				p.logger.Printf("[ERROR] failed to validate commit message from %s. Error: %v", msg.From, err)
				continue
			}
			p.state.addCommitted(msg)
		default:
			panic(fmt.Errorf("BUG: Unexpected message type: %s in %s", msg.Type, p.getState()))
		}

		//if voting power is disabled - count number of votes
		if !p.IsVotingPowerEnabled() {
			if p.state.numPrepared() > p.state.NumValid() {
				// we have received enough prepare messages
				sendCommit(span)
			}

			if p.state.numCommitted() > p.state.NumValid() {
				// we have received enough commit messages
				sendCommit(span)

				// change to commit state just to get out of the loop
				p.setState(CommitState)
			}
		} else {
			totalVotingPower := TotalVotingPower(p.config.VotingPower)
			validVotingPower := QuorumSizeVP(totalVotingPower)

			if p.state.calculateMessagesVotingPower(p.state.prepared, p.config.VotingPower) >= validVotingPower {
				// we have received enough prepare messages
				sendCommit(span)
			}

			if p.state.calculateMessagesVotingPower(p.state.committed, p.config.VotingPower) >= validVotingPower {
				// we have received enough commit messages
				sendCommit(span)

				// change to commit state just to get out of the loop
				p.setState(CommitState)
			}
		}
	}
}

// spanAddEventMessage reports given message to both PBFT built-in statistics reporting mechanism and open telemetry
func (p *Pbft) spanAddEventMessage(typ string, span trace.Span, msg *MessageReq) {
	p.stats.IncrMsgCount(msg.Type.String())

	span.AddEvent("Message", trace.WithAttributes(
		// where was the message generated
		attribute.String("typ", typ),

		// type of message
		attribute.String("msg", msg.Type.String()),

		// from address of the sender
		attribute.String("from", string(msg.From)),

		// view sequence
		attribute.Int64("sequence", int64(msg.View.Sequence)),

		// round sequence
		attribute.Int64("round", int64(msg.View.Round)),
	))
}

func (p *Pbft) setStateSpanAttributes(span trace.Span) {
	attr := []attribute.KeyValue{}

	// round
	attr = append(attr, attribute.Int64("round", int64(p.state.view.Round)))

	// number of commit messages
	attr = append(attr, attribute.Int("committed", p.state.numCommitted()))

	// number of prepare messages
	attr = append(attr, attribute.Int("prepared", p.state.numPrepared()))

	// number of change state messages per round
	for round, msgs := range p.state.roundMessages {
		attr = append(attr, attribute.Int64(fmt.Sprintf("roundchange_%d", round), int64(len(msgs))))
	}
	span.SetAttributes(attr...)
}

// resetRoundChangeSpan terminates previous span (if any) and starts a new one
func (p *Pbft) resetRoundChangeSpan(span trace.Span, ctx context.Context, iteration int64) trace.Span {
	if span != nil {
		span.End()
	}
	_, span = p.tracer.Start(ctx, "RoundChangeState")
	span.SetAttributes(attribute.Int64("iteration", iteration))
	return span
}

func (p *Pbft) runCommitState(ctx context.Context) {
	_, span := p.tracer.Start(ctx, "CommitState")
	defer span.End()

	committedSeals := p.state.getCommittedSeals()
	proposal := p.state.proposal.Copy()

	pp := &SealedProposal{
		Proposal:       proposal,
		CommittedSeals: committedSeals,
		Proposer:       p.state.proposer,
		Number:         p.state.view.Sequence,
	}
	if err := p.backend.Insert(pp); err != nil {
		// start a new round with the state unlocked since we need to
		// be able to propose/validate a different proposal
		p.logger.Printf("[ERROR] failed to insert proposal. Error message: %v", err)
		p.handleStateErr(errFailedToInsertProposal)
	} else {
		// move to done state to finish the current iteration of the state machine
		p.setState(DoneState)
	}
}

var (
	errIncorrectLockedProposal = fmt.Errorf("locked proposal is incorrect")
	errVerificationFailed      = fmt.Errorf("proposal verification failed")
	errFailedToInsertProposal  = fmt.Errorf("failed to insert proposal")
)

func (p *Pbft) handleStateErr(err error) {
	p.state.err = err
	p.setState(RoundChangeState)
}

func (p *Pbft) runRoundChangeState(ctx context.Context) {
	iteration := int64(1)
	span := p.resetRoundChangeSpan(nil, ctx, iteration)

	sendRoundChange := func(round uint64) {
		p.logger.Printf("[DEBUG] local round change: round=%d", round)
		// set the new round
		p.setRound(round)
		// set state attributes to the span
		p.setStateSpanAttributes(span)
		// clean the round
		p.state.cleanRound(round)
		// send the round change message
		p.sendRoundChange()
		// terminate a span and start a new one
		iteration++
		span = p.resetRoundChangeSpan(span, ctx, iteration)
	}

	sendNextRoundChange := func() {
		sendRoundChange(p.state.GetCurrentRound() + 1)
	}

	checkTimeout := func() {
		// At this point we might be stuck in the network if:
		// - We have advanced the round but everyone else passed.
		// - We are removing those messages since they are old now.
		if bestHeight, stucked := p.backend.IsStuck(p.state.view.Sequence); stucked {
			span.AddEvent("OutOfSync", trace.WithAttributes(
				// our local height
				attribute.Int64("local", int64(p.state.view.Sequence)),
				// the best remote height
				attribute.Int64("remote", int64(bestHeight)),
			))
			// set state span attributes and terminate it
			p.setStateSpanAttributes(span)
			span.End()
			p.setState(SyncState)
			return
		}

		// otherwise, it seems that we are in sync
		// and we should start a new round
		sendNextRoundChange()
	}

	// if the round was triggered due to an error, we send our own
	// next round change
	if err := p.state.getErr(); err != nil {
		p.logger.Printf("[DEBUG] round change handle error. Error message: %v", err)
		sendNextRoundChange()
	} else {
		// otherwise, it is due to a timeout in any stage
		// First, we try to sync up with any max round already available
		if maxRound, ok := p.state.maxRound(); ok {
			p.logger.Printf("[DEBUG] round change, max round=%d", maxRound)
			sendRoundChange(maxRound)
		} else {
			// otherwise, do your best to sync up
			checkTimeout()
		}
	}

	// create a timer for the round change
	for p.getState() == RoundChangeState {
		msg, ok := p.getNextMessage(span)
		if !ok {
			// closing
			return
		}
		if msg == nil {
			p.logger.Print("[DEBUG] round change timeout")

			// checkTimeout will either produce a sync event and exit
			// or restart the timeout
			checkTimeout()
			continue
		}

		// we only expect RoundChange messages right now
		num := p.state.AddRoundMessage(msg)

		//if voting power is disabled - count number of votes
		if !p.IsVotingPowerEnabled() {
			if num == p.state.NumValid() {
				// start a new round immediately
				p.state.SetCurrentRound(msg.View.Round)
				// set state span attributes and terminate it
				p.setStateSpanAttributes(span)
				span.End()
				p.setState(AcceptState)
			} else if num == p.state.MaxFaultyNodes()+1 {
				// weak certificate, try to catch up if our round number is smaller
				if p.state.GetCurrentRound() < msg.View.Round {
					// update timer
					sendRoundChange(msg.View.Round)
				}
			}
		} else {
			totalVotingPower := TotalVotingPower(p.config.VotingPower)
			validVotingPower := QuorumSizeVP(totalVotingPower)
			roundVotingPower := p.state.calculateMessagesVotingPower(p.state.roundMessages[msg.View.Round], p.config.VotingPower)
			if roundVotingPower >= validVotingPower {
				// start a new round immediately
				p.state.SetCurrentRound(msg.View.Round)
				// set state span attributes and terminate it
				p.setStateSpanAttributes(span)
				span.End()
				p.setState(AcceptState)
			} else if roundVotingPower >= MaxFaultyVP(totalVotingPower)+1 {
				// weak certificate, try to catch up if our round number is smaller
				if p.state.GetCurrentRound() < msg.View.Round {
					// update timer
					sendRoundChange(msg.View.Round)
				}
			}
		}
	}
}

// --- communication wrappers ---

func (p *Pbft) sendRoundChange() {
	p.gossip(MessageReq_RoundChange)
}

func (p *Pbft) sendPreprepareMsg() {
	p.gossip(MessageReq_Preprepare)
}

func (p *Pbft) sendPrepareMsg() {
	p.gossip(MessageReq_Prepare)
}

func (p *Pbft) sendCommitMsg() {
	p.gossip(MessageReq_Commit)
}

func (p *Pbft) gossip(msgType MsgType) {
	msg := &MessageReq{
		Type: msgType,
		From: p.validator.NodeID(),
	}
	if msgType != MessageReq_RoundChange {
		// Except for round change message in which we are deciding on the proposer,
		// the rest of the consensus message require the hash:
		// 1. Preprepare: notify the validators of the proposal + hash
		// 2. Prepare + Commit: safe check to only include messages from our round.
		msg.Hash = p.state.proposal.Hash
	}

	// add View
	msg.View = p.state.view.Copy()

	// if we are sending a preprepare message we need to include the proposal
	if msg.Type == MessageReq_Preprepare {
		msg.SetProposal(p.state.proposal.Data)
	}

	// if the message is commit, we need to add the committed seal
	if msg.Type == MessageReq_Commit {
		// seal the hash of the proposal
		seal, err := p.validator.Sign(p.state.proposal.Hash)
		if err != nil {
			p.logger.Printf("[ERROR] failed to commit seal. Error message: %v", err)
			return
		}
		msg.Seal = seal
	}

	if msg.Type != MessageReq_Preprepare {
		// send a copy to ourselves so that we can process this message as well
		msg2 := msg.Copy()
		msg2.From = p.validator.NodeID()
		p.PushMessage(msg2)
	}

	p.logger.Printf("[TRACE] gossip message: type=%s, sequence=%d, round=%d", msg.Type.String(), msg.View.Sequence, msg.View.Round)
	if err := p.transport.Gossip(msg); err != nil {
		p.logger.Printf("[ERROR] failed to gossip. Error message: %v", err)
	}
}

// GetValidatorId returns validator NodeID
func (p *Pbft) GetValidatorId() NodeID {
	return p.validator.NodeID()
}

// GetState returns the current PBFT state
func (p *Pbft) GetState() State {
	return p.getState()
}

// getState returns the current PBFT state
func (p *Pbft) getState() State {
	return p.state.getState()
}

// IsState checks if the node is in the passed in state
func (p *Pbft) IsState(s State) bool {
	return p.state.getState() == s
}

func (p *Pbft) SetState(s State) {
	p.setState(s)
}

// setState sets the PBFT state
func (p *Pbft) setState(s State) {
	p.logger.Printf("[DEBUG] state change: '%s'", s)
	p.state.setState(s)
}

// IsLocked returns if the current proposal is locked
func (p *Pbft) IsLocked() bool {
	return atomic.LoadUint64(&p.state.locked) == 1
}

// GetProposal returns current proposal in the pbft
func (p *Pbft) GetProposal() *Proposal {
	return p.state.proposal
}

func (p *Pbft) Round() uint64 {
	return atomic.LoadUint64(&p.state.view.Round)
}

// getNextMessage reads a new message from the message queue
func (p *Pbft) getNextMessage(span trace.Span) (*MessageReq, bool) {
	for {
		msg, discards := p.notifier.ReadNextMessage(p)
		// send the discard messages
		p.logger.Printf("[TRACE] Current state %s, number of prepared messages: %d, number of committed messages %d", p.getState(), p.state.numPrepared(), p.state.numCommitted())

		for _, msg := range discards {
			p.logger.Printf("[TRACE] Discarded %s ", msg)
			p.spanAddEventMessage("dropMessage", span, msg)
		}
		if msg != nil {
			// add the event to the span
			p.spanAddEventMessage("message", span, msg)
			p.logger.Printf("[TRACE] Received %s", msg)
			return msg, true
		}

		// wait until there is a new message or
		// someone closes the stopCh (i.e. timeout for round change)
		select {
		case <-p.state.timeoutChan:
			span.AddEvent("Timeout")
			p.notifier.HandleTimeout(p.validator.NodeID(), stateToMsg(p.getState()), &View{
				Round:    p.state.GetCurrentRound(),
				Sequence: p.state.view.Sequence,
			})
			p.logger.Printf("[TRACE] Message read timeout occurred")
			return nil, true
		case <-p.ctx.Done():
			return nil, false
		case <-p.updateCh:
		}
	}
}

func (p *Pbft) PushMessageInternal(msg *MessageReq) {
	p.msgQueue.pushMessage(msg)

	select {
	case p.updateCh <- struct{}{}:
	default:
	}
}

// PushMessage pushes a new message to the message queue
func (p *Pbft) PushMessage(msg *MessageReq) {
	if err := msg.Validate(); err != nil {
		p.logger.Printf("[ERROR] failed to validate msg: %v. Error: %v", msg, err)
		return
	}

	p.logger.Printf("[TRACE] receive message: type=%s, sequence=%d, round=%d", msg.Type.String(), msg.View.Sequence, msg.View.Round)
	p.PushMessageInternal(msg)
}

// ReadMessageWithDiscards reads next message with discards from message queue based on current state, sequence and round
func (p *Pbft) ReadMessageWithDiscards() (*MessageReq, []*MessageReq) {
	return p.msgQueue.readMessageWithDiscards(p.getState(), p.state.view)
}

func (p *Pbft) IsVotingPowerEnabled() bool {
	return p.config.VotingPower != nil
}

func ConsensusStateHandler(consensus *Pbft) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		res := map[string]interface{}{
			"state":         consensus.state.state,
			"proposal":      consensus.state.proposal,
			"view":          consensus.state.view,
			"prepared":      consensus.state.prepared,
			"committed":     consensus.state.committed,
			"roundMessages": consensus.state.roundMessages,
		}
		err := json.NewEncoder(writer).Encode(res)
		if err != nil {
			consensus.logger.Print("dump handler err", err)
			writer.WriteHeader(400)
		}
	}
}
