package ibft

import (
	"fmt"
	"math"
	"sync/atomic"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/types"
)

type IbftState uint32

const (
	AcceptState IbftState = iota
	RoundChangeState
	ValidateState
	CommitState
	SyncState
)

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

type currentState struct {
	// the snapshot being currently used
	validators ValidatorSet

	// state is the current state
	state uint64

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

func newState() *currentState {
	c := &currentState{}
	c.resetRoundMsgs()
	return c
}

func (c *currentState) setView(v *proto.View) {
	c.view = v
}

func (c *currentState) getState() IbftState {
	stateAddr := (*uint64)(&c.state)
	return IbftState(atomic.LoadUint64(stateAddr))
}

func (c *currentState) setState(s IbftState) {
	stateAddr := (*uint64)(&c.state)
	atomic.StoreUint64(stateAddr, uint64(s))
}

func (c *currentState) NumValid() int {
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
	obj, ok := c.roundMessages[round]
	if !ok {
		return 0
	}
	return len(obj)
}

func (c *currentState) AddRoundMessage(msg *proto.MessageReq) int {
	if msg.Type != proto.MessageReq_RoundChange {
		return 0
	}
	c.addMessage(msg)
	return len(c.roundMessages[msg.View.Round])
}

func (c *currentState) addPrepared(msg *proto.MessageReq) {
	if msg.Type != proto.MessageReq_Prepare {
		return
	}
	c.addMessage(msg)
}

func (c *currentState) addCommited(msg *proto.MessageReq) {
	if msg.Type != proto.MessageReq_Commit {
		return
	}
	c.addMessage(msg)
}

func (c *currentState) addMessage(msg *proto.MessageReq) {
	addr := msg.FromAddr()
	if !c.validators.Includes(addr) {
		// only include messages from validators
		return
	}

	if msg.Type == proto.MessageReq_Commit {
		c.committed[addr] = msg
	} else if msg.Type == proto.MessageReq_Prepare {
		c.prepared[addr] = msg
	} else if msg.Type == proto.MessageReq_RoundChange {
		view := msg.View
		if _, ok := c.roundMessages[view.Round]; !ok {
			c.roundMessages[view.Round] = map[types.Address]*proto.MessageReq{}
		}
		c.roundMessages[view.Round][addr] = msg
	}
}

func (c *currentState) numPrepared() int {
	return len(c.prepared)
}

func (c *currentState) numCommited() int {
	return len(c.committed)
}

type ValidatorSet []types.Address

func (v *ValidatorSet) CalcProposer(round uint64, lastProposer types.Address) types.Address {
	seed := uint64(0)
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

func (v *ValidatorSet) Add(addr types.Address) {
	*v = append(*v, addr)
}

func (v *ValidatorSet) Del(addr types.Address) {
	for indx, i := range *v {
		if i == addr {
			*v = append((*v)[:indx], (*v)[indx+1:]...)
		}
	}
}

func (v *ValidatorSet) Len() int {
	return len(*v)
}

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

func (v *ValidatorSet) Index(addr types.Address) int {
	for indx, i := range *v {
		if i == addr {
			return indx
		}
	}
	return -1
}

func (v *ValidatorSet) Includes(addr types.Address) bool {
	return v.Index(addr) != -1
}

func (v *ValidatorSet) MinFaultyNodes() int {
	return int(math.Ceil(float64(len(*v))/3)) - 1
}
