package ibft

import (
	"sync"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
)

// new messages arrive from the network. Each message belongs
// to a Round and Sequence. We need to consume them sorted. First order
// is the sequence, then the round and then the type of message
// in order (preprepare, commit, prepare).

type queue struct {
	lock     sync.Mutex
	queue    queueImpl
	updateCh chan struct{}
}

func newQueue() *queue {
	return &queue{
		queue:    queueImpl{},
		updateCh: make(chan struct{}),
	}
}

func (q *queue) add(obj *proto.MessageReq) {

}

const (
	// priority order for the messages
	msgPreprepare = uint64(1)
	msgCommit     = uint64(2)
	msgPrepare    = uint64(3)
)

type queueTask struct {
	// heap objects
	index int

	// priority
	sequence uint64
	round    uint64
	msg      uint64

	// raw
	obj interface{}
}

type queueImpl []*queueTask

func (t queueImpl) Len() int { return len(t) }

func (t queueImpl) Less(i, j int) bool {
	ti, tj := t[i], t[j]
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

func (t queueImpl) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].index = i
	t[j].index = j
}

func (t *queueImpl) Push(x interface{}) {
	n := len(*t)
	item := x.(*queueTask)
	item.index = n
	*t = append(*t, item)
}

func (t *queueImpl) Pop() interface{} {
	old := *t
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*t = old[0 : n-1]
	return item
}
