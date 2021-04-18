package ibft

import (
	"container/heap"
	"sync"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
)

type msgQueue struct {
	roundChangeStateQueue msgQueueImpl
	acceptStateQueue      msgQueueImpl
	validateStateQueue    msgQueueImpl

	queueLock sync.Mutex
}

func (m *msgQueue) pushMessage(task *msgTask) {
	m.queueLock.Lock()
	queue := m.getQueue(msgToState(task.msg))
	heap.Push(queue, task)
	m.queueLock.Unlock()
}

func (m *msgQueue) readMessage(state IbftState, current *proto.View) *msgTask {
	m.queueLock.Lock()
	defer m.queueLock.Unlock()

	queue := m.getQueue(state)

	for {
		if queue.Len() == 0 {
			return nil
		}
		msg := queue.head()

		// check if the message is from the future
		if state == RoundChangeState {
			// if we are in RoundChangeState we only care about sequence
			// since we are interested in knowing all the possible rounds
			if msg.view.Sequence > current.Sequence {
				// future message
				return nil
			}
		} else {
			// otherwise, we compare both sequence and round
			if cmpView(msg.view, current) > 0 {
				// future message
				return nil
			}
		}

		// at this point, 'msg' is good or old, in either case
		// we have to remove it from the queue
		heap.Pop(queue)

		if cmpView(msg.view, current) < 0 {
			// old value, try again
			continue
		}

		// good value, return it
		return msg
	}
}

func (m *msgQueue) getQueue(state IbftState) *msgQueueImpl {
	if state == RoundChangeState {
		// round change
		return &m.roundChangeStateQueue
	} else if state == AcceptState {
		// preprepare
		return &m.acceptStateQueue
	} else {
		// prepare and commit
		return &m.validateStateQueue
	}
}

func newMsgQueue() *msgQueue {
	return &msgQueue{
		roundChangeStateQueue: msgQueueImpl{},
		acceptStateQueue:      msgQueueImpl{},
		validateStateQueue:    msgQueueImpl{},
	}
}

func protoTypeToMsg(typ proto.MessageReq_Type) MsgType {
	if typ == proto.MessageReq_Preprepare {
		return msgPreprepare
	} else if typ == proto.MessageReq_Prepare {
		return msgPrepare
	} else if typ == proto.MessageReq_Commit {
		return msgCommit
	}
	return msgRoundChange
}

func msgToState(msg MsgType) IbftState {
	if msg == msgRoundChange {
		// round change
		return RoundChangeState
	} else if msg == msgPreprepare {
		// preprepare
		return AcceptState
	} else if msg == msgPrepare || msg == msgCommit {
		// prepare and commit
		return ValidateState
	}
	panic("BUG: not expected")
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

func (m msgQueueImpl) head() *msgTask {
	return m[0]
}

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
