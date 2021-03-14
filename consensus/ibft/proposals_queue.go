package ibft

import (
	"fmt"
	"sync"

	"github.com/0xPolygon/minimal/types"
)

// Proposal2 is a new block proposal
type Proposal2 struct {
	index int

	Block types.Block
}

type proposalsQueue struct {
	lock     sync.Mutex
	updateCh chan struct{}
	queue    proposalsQueueImpl
}

func newProposalsQueue() *proposalsQueue {
	return &proposalsQueue{
		updateCh: make(chan struct{}),
	}
}

func (r *proposalsQueue) Add(b types.Block) {
	r.lock.Lock()
	defer r.lock.Unlock()

	p := &Proposal2{}
	fmt.Println(p)
}

func (r *proposalsQueue) Accept() {

}

func (r *proposalsQueue) GetProposal() *Proposal2 {
	r.lock.Lock()
	return nil
}

type proposalsQueueImpl []*Proposal2

func (p proposalsQueueImpl) Len() int { return len(p) }

func (p proposalsQueueImpl) Less(i, j int) bool {
	return p[i].Block.Number() < p[j].Block.Number()
}

func (p proposalsQueueImpl) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
	p[i].index = i
	p[j].index = j
}

func (p *proposalsQueueImpl) Push(x interface{}) {
	n := len(*p)
	item := x.(*Proposal2)
	item.index = n
	*p = append(*p, item)
}

func (p *proposalsQueueImpl) Pop() interface{} {
	old := *p
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*p = old[0 : n-1]
	return item
}
