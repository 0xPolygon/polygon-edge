package sampool

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/rootchain"
)

var (
	ErrInvalidHash      = errors.New("invalid SAM hash")
	ErrInvalidSignature = errors.New("invalid SAM signature")
	ErrStaleMessage     = errors.New("stale message received")
)

//	Verifies hash and signature of a SAM
type Verifier interface {
	VerifyHash(rootchain.SAM) error
	VerifySignature(rootchain.SAM) error

	Quorum(uint64) bool
}

type SAMPool struct {
	verifier Verifier

	messages map[uint64]samBucket

	lastProcessedMessage uint64
}

func New(verifier Verifier) *SAMPool {
	return &SAMPool{
		verifier: verifier,
		messages: make(map[uint64]samBucket),
	}
}

func (p *SAMPool) AddMessage(msg rootchain.SAM) error {
	//	verify message hash
	if err := p.verifier.VerifyHash(msg); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidHash, err)
	}

	//	verify message signature
	if err := p.verifier.VerifySignature(msg); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}

	//	reject old message
	msgNumber := msg.Event.Number
	if msgNumber <= p.lastProcessedMessage {
		return fmt.Errorf("%w: message number %d", ErrStaleMessage, msgNumber)
	}

	//	add message

	//	TODO: lock/unlock here
	bucket := p.messages[msgNumber]
	if bucket == nil {
		bucket = newBucket()
		p.messages[msgNumber] = bucket
	}

	bucket.add(msg)

	return nil
}

func (p *SAMPool) Prune(index uint64) {
	//	TODO: lock/unlock

	for idx := range p.messages {
		if idx <= index {
			delete(p.messages, idx)
		}
	}

	p.lastProcessedMessage = index

}

//	TODO: Peek or Pop might be redundant

func (p *SAMPool) Peek() rootchain.VerifiedSAM {
	//	TODO: lock/unlock

	expectedMessageNumber := p.lastProcessedMessage + 1

	bucket := p.messages[expectedMessageNumber]
	if bucket == nil {
		return nil
	}

	return bucket.getReadyMessages(p.verifier.Quorum)

}

func (p *SAMPool) Pop() rootchain.VerifiedSAM {
	return nil
}
