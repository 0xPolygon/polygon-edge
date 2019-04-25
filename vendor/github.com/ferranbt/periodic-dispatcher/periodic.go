package periodic

import (
	"container/heap"
	"fmt"
	"sync"
	"time"
)

// Job is a job in the dispatcher
type Job interface {
	ID() string
}

type pJob struct {
	job    Job
	period time.Duration
}

// Dispatcher is used to track and launch pJobs
type Dispatcher struct {
	heap    *periodicHeap
	tracked map[string]*pJob

	enabled bool

	eventCh  chan Job
	updateCh chan struct{}
	cancelCh chan struct{}

	l sync.RWMutex
}

// NewDispatcher creates a new dispatcher
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		tracked:  make(map[string]*pJob),
		heap:     newPeriodicHeap(),
		enabled:  false,
		updateCh: make(chan struct{}, 1),
		eventCh:  make(chan Job, 10),
		l:        sync.RWMutex{},
	}
}

// Events returns the channel of events
func (p *Dispatcher) Events() chan Job {
	return p.eventCh
}

// Contains check if the job is on the dispatcher
func (p *Dispatcher) Contains(id string) bool {
	p.l.Lock()
	defer p.l.Unlock()

	_, ok := p.tracked[id]
	return ok
}

// Add adds a new job with an interval period to dispatch the job
func (p *Dispatcher) Add(job Job, period time.Duration) error {
	p.l.Lock()
	defer p.l.Unlock()

	pJob := &pJob{job, period}

	// Add or update the pJob.
	p.tracked[pJob.job.ID()] = pJob

	next := time.Now().Add(pJob.period)
	if err := p.heap.Push(pJob, next); err != nil {
		return err
	}

	// Signal an update.
	select {
	case p.updateCh <- struct{}{}:
	default:
	}

	return nil
}

// SetEnabled is used to control if the periodic dispatcher is enabled
func (p *Dispatcher) SetEnabled(enabled bool) {
	p.l.Lock()
	defer p.l.Unlock()
	wasRunning := p.enabled
	p.enabled = enabled

	if !enabled && wasRunning {
		close(p.cancelCh)
	} else if enabled && !wasRunning {
		p.cancelCh = make(chan struct{})
		go p.run()
	}
}

// Remove removes a job from the dispatcher
func (p *Dispatcher) Remove(value string) error {
	p.l.Lock()
	defer p.l.Unlock()

	pJob, tracked := p.tracked[value]
	if !tracked {
		return nil
	}

	delete(p.tracked, value)
	if err := p.heap.Remove(pJob.job.ID()); err != nil {
		return fmt.Errorf("failed to remove tracked pJob (%s): %v", value, err)
	}

	// Signal an update.
	select {
	case p.updateCh <- struct{}{}:
	default:
	}

	return nil
}

// Tracked returns the object being tracked
func (p *Dispatcher) Tracked() []Job {
	p.l.RLock()
	defer p.l.RUnlock()

	tracked := make([]Job, len(p.tracked))
	i := 0
	for _, job := range p.tracked {
		tracked[i] = job.job
		i++
	}
	return tracked
}

func (p *Dispatcher) run() {
	var launchCh <-chan time.Time
	for {
		pJob, launch := p.nextLaunch()
		if launch.IsZero() {
			launchCh = nil
		} else {
			launchDur := launch.Sub(time.Now())
			launchCh = time.After(launchDur)
		}

		select {
		case <-p.cancelCh:
			return
		case <-p.updateCh:
			continue
		case <-launchCh:
			p.dispatch(pJob, launch)
		}
	}
}

func (p *Dispatcher) dispatch(pJob *pJob, launch time.Time) {
	p.l.Lock()
	defer p.l.Unlock()

	nextLaunch := launch.Add(pJob.period)

	if err := p.heap.Update(pJob.job.ID(), nextLaunch); err != nil {
		// TODO. handle error
	}

	select {
	case p.eventCh <- pJob.job:
	default:
	}
}

func (p *Dispatcher) nextLaunch() (*pJob, time.Time) {
	p.l.RLock()
	defer p.l.RUnlock()

	if p.heap.Length() == 0 {
		return nil, time.Time{}
	}

	nextpJob := p.heap.Peek()
	if nextpJob == nil {
		return nil, time.Time{}
	}

	return nextpJob.value, nextpJob.next
}

// --- periodic heap ---

type periodicHeap struct {
	index map[string]*periodicpJob
	heap  periodicHeapImp
}

type periodicpJob struct {
	id    string // the index
	value *pJob
	next  time.Time
	index int
}

func newPeriodicHeap() *periodicHeap {
	return &periodicHeap{
		index: make(map[string]*periodicpJob),
		heap:  make(periodicHeapImp, 0),
	}
}

func (p *periodicHeap) Push(pJob *pJob, next time.Time) error {
	if _, ok := p.index[pJob.job.ID()]; ok {
		return fmt.Errorf("job (%s) already exists", pJob.job.ID())
	}

	ppJob := &periodicpJob{pJob.job.ID(), pJob, next, 0}
	p.index[pJob.job.ID()] = ppJob
	heap.Push(&p.heap, ppJob)
	return nil
}

func (p *periodicHeap) Pop() *periodicpJob {
	if len(p.heap) == 0 {
		return nil
	}

	ppJob := heap.Pop(&p.heap).(*periodicpJob)
	delete(p.index, ppJob.id)
	return ppJob
}

func (p *periodicHeap) Peek() *periodicpJob {
	if len(p.heap) == 0 {
		return nil
	}

	return p.heap[0]
}

func (p *periodicHeap) Contains(id string) bool {
	_, ok := p.index[id]
	return ok
}

func (p *periodicHeap) Update(id string, next time.Time) error {
	if ppJob, ok := p.index[id]; ok {
		ppJob.id = id
		ppJob.next = next
		heap.Fix(&p.heap, ppJob.index)
		return nil
	}

	return fmt.Errorf("heap doesn't contain pJob (%s)", id)
}

func (p *periodicHeap) Remove(id string) error {
	if ppJob, ok := p.index[id]; ok {
		heap.Remove(&p.heap, ppJob.index)
		delete(p.index, id)
		return nil
	}

	return fmt.Errorf("heap doesn't contain pJob (%s)", id)
}

func (p *periodicHeap) Length() int {
	return len(p.heap)
}

// --- periodic heap imp ---

type periodicHeapImp []*periodicpJob

func (h periodicHeapImp) Len() int {
	return len(h)
}

func (h periodicHeapImp) Less(i, j int) bool {
	iZero, jZero := h[i].next.IsZero(), h[j].next.IsZero()
	if iZero && jZero {
		return false
	} else if iZero {
		return false
	} else if jZero {
		return true
	}

	return h[i].next.Before(h[j].next)
}

func (h periodicHeapImp) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *periodicHeapImp) Push(x interface{}) {
	n := len(*h)
	pJob := x.(*periodicpJob)
	pJob.index = n
	*h = append(*h, pJob)
}

func (h *periodicHeapImp) Pop() interface{} {
	old := *h
	n := len(old)
	pJob := old[n-1]
	pJob.index = -1
	*h = old[0 : n-1]
	return pJob
}
