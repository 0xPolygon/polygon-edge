package sampool

import (
	"errors"
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/rootchain"
)

var (
	ErrInvalidHash      = errors.New("invalid SAM hash")
	ErrInvalidSignature = errors.New("invalid SAM signature")
	ErrStaleMessage     = errors.New("stale SAM received")
)

type Verifier interface {
	VerifyHash(rootchain.SAM) error
	VerifySignature(rootchain.SAM) error

	Quorum(uint64) bool
}

type SAMPool struct {
	// TODO: remove once milos moves verification to SAMUEL
	verifier Verifier

	mux      sync.Mutex
	messages map[uint64]samBucket

	lastProcessedIndex uint64
}

func New(verifier Verifier) *SAMPool {
	return &SAMPool{
		verifier: verifier,
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

// TODO: Peek or Pop might be redundant
func (p *SAMPool) Peek() rootchain.VerifiedSAM {
	p.mux.Lock()
	defer p.mux.Unlock()

	expectedIndex := p.lastProcessedIndex + 1

	bucket := p.messages[expectedIndex]
	if bucket == nil {
		return nil
	}

	messages := bucket.getQuorumMessages(p.verifier.Quorum)
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

	messages := bucket.getQuorumMessages(p.verifier.Quorum)
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
	//	verify message hash
	if err := p.verifier.VerifyHash(msg); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidHash, err)
	}

	//	verify message signature
	if err := p.verifier.VerifySignature(msg); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}

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
