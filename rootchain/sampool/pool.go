package sampool

import (
	"errors"
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/rootchain"
)

var (
	ErrStaleMessage = errors.New("stale SAM received")
)

type SAMPool struct {
	mux sync.Mutex

	messages           map[uint64]samBucket
	lastProcessedIndex uint64
}

func New() *SAMPool {
	return &SAMPool{
		mux:      sync.Mutex{},
		messages: make(map[uint64]samBucket),
	}
}

func (p *SAMPool) AddMessage(msg rootchain.SAM) error {
	if err := p.verifySAM(msg); err != nil {
		return err
	}

	p.addSAM(msg)

	return nil
}

func (p *SAMPool) SetLastProcessedEvent(index uint64) {
	p.mux.Lock()
	defer p.mux.Unlock()

	p.lastProcessedIndex = index
}

func (p *SAMPool) Prune(index uint64) {
	p.mux.Lock()
	defer p.mux.Unlock()

	for idx := range p.messages {
		if idx <= index {
			delete(p.messages, idx)
		}
	}

	p.lastProcessedIndex = index
}

func (p *SAMPool) Peek() rootchain.VerifiedSAM {
	p.mux.Lock()
	defer p.mux.Unlock()

	expectedIndex := p.lastProcessedIndex + 1

	bucket := p.messages[expectedIndex]
	if bucket == nil {
		return nil
	}

	messages := bucket.getMessagesWithMostSignatures()
	if len(messages) == 0 {
		return nil
	}

	result := make([]rootchain.SAM, len(messages))
	copy(result, messages)

	return result
}

func (p *SAMPool) Pop() rootchain.VerifiedSAM {
	p.mux.Lock()
	defer p.mux.Unlock()

	expectedIndex := p.lastProcessedIndex + 1

	bucket := p.messages[expectedIndex]
	if bucket == nil {
		return nil
	}

	messages := bucket.getMessagesWithMostSignatures()
	if len(messages) == 0 {
		return nil
	}

	//	remove associated bucket
	delete(p.messages, expectedIndex)

	//	update index for next call
	p.lastProcessedIndex = expectedIndex

	return messages
}

func (p *SAMPool) verifySAM(msg rootchain.SAM) error {
	//	reject old message
	if index := msg.Index; index <= p.lastProcessedIndex {
		return fmt.Errorf("%w: message number %d", ErrStaleMessage, index)
	}

	return nil
}

func (p *SAMPool) addSAM(msg rootchain.SAM) {
	p.mux.Lock()
	defer p.mux.Unlock()

	var (
		index  = msg.Index
		bucket = p.messages[index]
	)

	if bucket == nil {
		bucket = newBucket()
		p.messages[index] = bucket
	}

	bucket.add(msg)
}
