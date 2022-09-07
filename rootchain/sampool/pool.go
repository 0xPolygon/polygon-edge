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
	verifier Verifier

	mux      sync.Mutex
	messages map[uint64]samBucket

	lastProcessedMessage uint64
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

	p.lastProcessedMessage = index
}

// TODO: Peek or Pop might be redundant
func (p *SAMPool) Peek() rootchain.VerifiedSAM {
	p.mux.Lock()
	defer p.mux.Unlock()

	expectedMessageNumber := p.lastProcessedMessage + 1

	bucket := p.messages[expectedMessageNumber]
	if bucket == nil {
		return nil
	}

	return bucket.getQuorumMessages(p.verifier.Quorum)
}

func (p *SAMPool) Pop() rootchain.VerifiedSAM {
	return nil
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
	if msgNumber := msg.Event.Index; msgNumber <= p.lastProcessedMessage {
		return fmt.Errorf("%w: message number %d", ErrStaleMessage, msgNumber)
	}

	return nil
}

func (p *SAMPool) addSAM(msg rootchain.SAM) {
	p.mux.Lock()
	defer p.mux.Unlock()

	var (
		msgNumber = msg.Index
		bucket    = p.messages[msgNumber]
	)

	if bucket == nil {
		bucket = newBucket()
		p.messages[msgNumber] = bucket
	}

	bucket.add(msg)
}
