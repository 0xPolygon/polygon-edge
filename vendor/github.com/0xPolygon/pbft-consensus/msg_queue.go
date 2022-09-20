package pbft

import (
	"container/heap"
	"sync"
)

// msgQueue defines the structure that holds message queues for different PBFT states
type msgQueue struct {
	// Heap implementation for the round change message queue
	roundChangeStateQueue msgQueueImpl

	// Heap implementation for the accept state message queue
	acceptStateQueue msgQueueImpl

	// Heap implementation for the validate state message queue
	validateStateQueue msgQueueImpl

	queueLock sync.Mutex
}

// pushMessage adds a new message to a message queue
func (m *msgQueue) pushMessage(message *MessageReq) {
	m.queueLock.Lock()
	defer m.queueLock.Unlock()

	queue := m.getQueue(msgToState(message.Type))
	heap.Push(queue, message)
}

// readMessage reads the message from a message queue, based on the current state and view
func (m *msgQueue) readMessage(st State, current *View) *MessageReq {
	msg, _ := m.readMessageWithDiscards(st, current)
	return msg
}

func (m *msgQueue) readMessageWithDiscards(st State, current *View) (*MessageReq, []*MessageReq) {
	m.queueLock.Lock()
	defer m.queueLock.Unlock()

	discarded := []*MessageReq{}
	queue := m.getQueue(st)

	for {
		if queue.Len() == 0 {
			return nil, discarded
		}
		msg := queue.head()

		// check if the message is from the future
		if st == RoundChangeState {
			// if we are in RoundChangeState we only care about sequence
			// since we are interested in knowing all the possible rounds
			if msg.View.Sequence > current.Sequence {
				// future message
				return nil, discarded
			}
		} else {
			// otherwise, we compare both sequence and round
			if cmpView(msg.View, current) > 0 {
				// future message
				return nil, discarded
			}
		}

		// at this point, 'msg' is good or old, in either case
		// we have to remove it from the queue
		heap.Pop(queue)

		if cmpView(msg.View, current) < 0 {
			// old value, try again
			discarded = append(discarded, msg)
			continue
		}

		// good value, return it
		return msg, discarded
	}
}

// getQueue checks the passed in state, and returns the corresponding message queue
func (m *msgQueue) getQueue(st State) *msgQueueImpl {
	if st == RoundChangeState {
		// round change
		return &m.roundChangeStateQueue
	} else if st == AcceptState {
		// preprepare
		return &m.acceptStateQueue
	} else {
		// prepare and commit
		return &m.validateStateQueue
	}
}

// newMsgQueue creates a new message queue structure
func newMsgQueue() *msgQueue {
	return &msgQueue{
		roundChangeStateQueue: msgQueueImpl{},
		acceptStateQueue:      msgQueueImpl{},
		validateStateQueue:    msgQueueImpl{},
	}
}

// msgToState converts the message type to an State
func msgToState(msg MsgType) State {
	if msg == MessageReq_RoundChange {
		// round change
		return RoundChangeState
	} else if msg == MessageReq_Preprepare {
		// preprepare
		return AcceptState
	} else if msg == MessageReq_Prepare || msg == MessageReq_Commit {
		// prepare and commit
		return ValidateState
	}

	panic("BUG: not expected")
}

func stateToMsg(st State) MsgType {
	switch st {
	case RoundChangeState:
		return MessageReq_RoundChange
	case AcceptState:
		return MessageReq_Preprepare
	case ValidateState:
		return MessageReq_Prepare
	default:
		panic("BUG: not expected")
	}
}

type msgQueueImpl []*MessageReq

// head returns the head of the queue
func (m msgQueueImpl) head() *MessageReq {
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
	if ti.View.Sequence != tj.View.Sequence {
		return ti.View.Sequence < tj.View.Sequence
	}
	// sort by round
	if ti.View.Round != tj.View.Round {
		return ti.View.Round < tj.View.Round
	}
	// sort by message
	return ti.Type < tj.Type
}

// Swap swaps the places of the items at the passed-in indexes
func (m msgQueueImpl) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// Push adds a new item to the queue
func (m *msgQueueImpl) Push(x interface{}) {
	*m = append(*m, x.(*MessageReq))
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

// cmpView compares two proto views.
//
// If v.Sequence == y.Sequence && v.Round == y.Round => 0
//
// If v.Sequence < y.Sequence => -1 ELSE => 1
//
// If v.Round < y.Round => -1 ELSE 1
func cmpView(v, y *View) int {
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
