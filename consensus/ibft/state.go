package ibft

import (
	"fmt"
	"math"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
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
	validators ValidatorSet

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

type ValidatorSet []types.Address

// CalcProposer calculates the address of the next proposer, from the validator set
func (v *ValidatorSet) CalcProposer(round uint64, lastProposer types.Address) types.Address {
	var seed uint64

	if lastProposer == types.ZeroAddress {
		seed = round
	} else {
		offset := 0
		if indx := v.Index(lastProposer); indx != -1 {
			offset = indx
		}

		seed = uint64(offset) + round + 1
	}

	pick := seed % uint64(v.Len())

	return (*v)[pick]
}

// Add adds a new address to the validator set
func (v *ValidatorSet) Add(addr types.Address) {
	*v = append(*v, addr)
}

// Del removes an address from the validator set
func (v *ValidatorSet) Del(addr types.Address) {
	for indx, i := range *v {
		if i == addr {
			*v = append((*v)[:indx], (*v)[indx+1:]...)
		}
	}
}

// Len returns the size of the validator set
func (v *ValidatorSet) Len() int {
	return len(*v)
}

// Equal checks if 2 validator sets are equal
func (v *ValidatorSet) Equal(vv *ValidatorSet) bool {
	if len(*v) != len(*vv) {
		return false
	}

	for indx := range *v {
		if (*v)[indx] != (*vv)[indx] {
			return false
		}
	}

	return true
}

// Index returns the index of the passed in address in the validator set.
// Returns -1 if not found
func (v *ValidatorSet) Index(addr types.Address) int {
	for indx, i := range *v {
		if i == addr {
			return indx
		}
	}

	return -1
}

// Includes checks if the address is in the validator set
func (v *ValidatorSet) Includes(addr types.Address) bool {
	return v.Index(addr) != -1
}

// MaxFaultyNodes returns the maximum number of allowed faulty nodes (F), based on the current validator set
func (v *ValidatorSet) MaxFaultyNodes() int {
	// N -> number of nodes in IBFT
	// F -> number of faulty nodes
	//
	// N = 3F + 1
	// => F = (N - 1) / 3
	//
	// IBFT tolerates 1 failure with 4 nodes
	// 4 = 3 * 1 + 1
	// To tolerate 2 failures, IBFT requires 7 nodes
	// 7 = 3 * 2 + 1
	// It should always take the floor of the result
	return (len(*v) - 1) / 3
}

type QuorumImplementation func(ValidatorSet) int

//	LegacyQuorumSize returns the legacy quorum size for the given validator set
func LegacyQuorumSize(set ValidatorSet) int {
	// According to the IBFT spec, the number of valid messages
	// needs to be 2F + 1
	return 2*set.MaxFaultyNodes() + 1
}

// OptimalQuorumSize returns the optimal quorum size for the given validator set
func OptimalQuorumSize(set ValidatorSet) int {
	//	if the number of validators is less than 4,
	//	then the entire set is required
	if set.MaxFaultyNodes() == 0 {
		/*
			N: 1 -> Q: 1
			N: 2 -> Q: 2
			N: 3 -> Q: 3
		*/
		return set.Len()
	}

	// (quorum optimal)	Q = ceil(2/3 * N)
	return int(math.Ceil(2 * float64(set.Len()) / 3))
}
