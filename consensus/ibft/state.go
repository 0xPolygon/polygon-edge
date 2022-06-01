package ibft

import (
	"fmt"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/validators"
	"github.com/0xPolygon/polygon-edge/types"
)

type IbftState uint32

// Define the states in IBFT
const (
	AcceptState IbftState = iota
	RoundChangeState
	ValidateState
	CommitState
	SyncState
)

// String returns the string representation of the passed in state
func (i IbftState) String() string {
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
	}

	panic(fmt.Sprintf("BUG: Ibft state not found %d", i))
}

// currentState defines the current state object in IBFT
type currentState struct {
	// validators represent the current validator set
	validators validators.ValidatorSet

	// state is the current state
	state uint64

	// The proposed block
	block *types.Block

	// The selected proposer
	proposer types.Address

	// Current view
	view *proto.View

	// List of prepared messages
	prepared map[types.Address]*proto.MessageReq

	// List of committed messages
	committed map[types.Address]*proto.MessageReq

	// List of round change messages
	roundMessages map[uint64]map[types.Address]*proto.MessageReq

	// Locked signals whether the proposal is locked
	locked bool

	// Describes whether there has been an error during the computation
	err error
}

// newState creates a new state with reset round messages
func newState() *currentState {
	c := &currentState{}
	c.resetRoundMsgs()

	return c
}

// getState returns the current state
func (c *currentState) getState() IbftState {
	stateAddr := &c.state

	return IbftState(atomic.LoadUint64(stateAddr))
}

// setState sets the current state
func (c *currentState) setState(s IbftState) {
	stateAddr := &c.state

	atomic.StoreUint64(stateAddr, uint64(s))
}

// getErr returns the current error, if any, and consumes it
func (c *currentState) getErr() error {
	err := c.err
	c.err = nil

	return err
}

func (c *currentState) maxRound() (maxRound uint64, found bool) {
	num := c.validators.MaxFaultyNodes() + 1

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

// resetRoundMsgs resets the prepared, committed and round messages in the current state
func (c *currentState) resetRoundMsgs() {
	c.prepared = map[types.Address]*proto.MessageReq{}
	c.committed = map[types.Address]*proto.MessageReq{}
	c.roundMessages = map[uint64]map[types.Address]*proto.MessageReq{}
}

// CalcProposer calculates the proposer and sets it to the state
func (c *currentState) CalcProposer(lastProposer types.Address) {
	c.proposer = c.validators.CalcProposer(c.view.Round, lastProposer)
}

func (c *currentState) lock() {
	c.locked = true
}

func (c *currentState) unlock() {
	c.block = nil
	c.locked = false
}

// cleanRound deletes the specific round messages
func (c *currentState) cleanRound(round uint64) {
	//	clear messages from previous round
	for r := range c.roundMessages {
		if r < round {
			delete(c.roundMessages, r)
		}
	}
}

// AddRoundMessage adds a message to the round, and returns the round message size
func (c *currentState) AddRoundMessage(msg *proto.MessageReq) int {
	if msg.Type != proto.MessageReq_RoundChange {
		return 0
	}

	c.addMessage(msg)

	return len(c.roundMessages[msg.View.Round])
}

// addPrepared adds a prepared message
func (c *currentState) addPrepared(msg *proto.MessageReq) {
	if msg.Type != proto.MessageReq_Prepare {
		return
	}

	c.addMessage(msg)
}

// addCommitted adds a committed message
func (c *currentState) addCommitted(msg *proto.MessageReq) {
	if msg.Type != proto.MessageReq_Commit {
		return
	}

	c.addMessage(msg)
}

// addMessage adds a new message to one of the following message lists: committed, prepared, roundMessages
func (c *currentState) addMessage(msg *proto.MessageReq) {
	addr := msg.FromAddr()
	if !c.validators.Includes(addr) {
		// only include messages from validators
		return
	}

	switch {
	case msg.Type == proto.MessageReq_Commit:
		c.committed[addr] = msg
	case msg.Type == proto.MessageReq_Prepare:
		c.prepared[addr] = msg
	case msg.Type == proto.MessageReq_RoundChange:
		view := msg.View
		if _, ok := c.roundMessages[view.Round]; !ok {
			c.roundMessages[view.Round] = map[types.Address]*proto.MessageReq{}
		}

		c.roundMessages[view.Round][addr] = msg
	}
}

// numPrepared returns the number of messages in the prepared message list
func (c *currentState) numPrepared() int {
	return len(c.prepared)
}

// numCommitted returns the number of messages in the committed message list
func (c *currentState) numCommitted() int {
	return len(c.committed)
}
