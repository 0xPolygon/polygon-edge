package sampool

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/rootchain"
)

var (
	ErrStaleMessage = errors.New("stale message received")
)

//	Verifies hash and signature of a SAM
type Verifier interface {
	VerifyHash(rootchain.SAM) error
	VerifySignature(rootchain.SAM) error

	Quorum(uint64) bool
}

type SAMPool struct {
	verifier Verifier

	messagesByNumber map[uint64]samBucket

	lastProcessedMessage uint64
}

func New(verifier Verifier) *SAMPool {
	return &SAMPool{
		verifier:         verifier,
		messagesByNumber: make(map[uint64]samBucket),
	}
}

func (s *SAMPool) AddMessage(msg rootchain.SAM) error {
	//	verify message hash
	if err := s.verifier.VerifyHash(msg); err != nil {
		return err
	}

	//	verify message signature
	if err := s.verifier.VerifySignature(msg); err != nil {
		return err
	}

	//	reject old message
	msgNumber := msg.Event.Number
	if msgNumber <= s.lastProcessedMessage {
		return fmt.Errorf("%w: message number %d", ErrStaleMessage, msgNumber)
	}

	//	add message

	//	TODO: lock/unlock here
	bucket := s.messagesByNumber[msgNumber]
	if bucket == nil {
		bucket = newBucket()
		s.messagesByNumber[msgNumber] = bucket
	}

	bucket.add(msg)

	return nil
}

func (s *SAMPool) Prune(index uint64) {

}

//	TODO: Peek or Pop might be redundant

func (s *SAMPool) Peek() rootchain.VerifiedSAM {
	return nil
}

func (s *SAMPool) Pop() rootchain.VerifiedSAM {
	return nil
}
