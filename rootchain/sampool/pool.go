package sampool

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"sync"

	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/polygon-edge/rootchain"
)

var (
	ErrStaleMessage = errors.New("stale SAM received")
)

// SAMPool is a storage for Signed Arbitrary Messages. Its main purpose is
// to aggregate signatures of received SAMs and preserve their ordering (SAM.Index)
type SAMPool struct {
	mux    sync.Mutex
	logger hclog.Logger

	messages           map[uint64]samBucket
	lastProcessedIndex uint64
}

// New returns a new SAMPool instance
func New(logger hclog.Logger) *SAMPool {
	return &SAMPool{
		logger:   logger,
		mux:      sync.Mutex{},
		messages: make(map[uint64]samBucket),
	}
}

// AddMessage adds the given message to the pool
func (p *SAMPool) AddMessage(msg rootchain.SAM) error {
	if err := p.verifySAM(msg); err != nil {
		p.logger.Error("add SAM failed", "err", err)

		return err
	}

	p.addSAM(msg)

	p.logger.Debug("added SAM", "index", msg.Index, "hash", msg.Hash.String())

	return nil
}

// SetLastProcessedEvent updates the SAMPool's internal index
// for keeping track of SAM ordering
func (p *SAMPool) SetLastProcessedEvent(index uint64) {
	p.mux.Lock()
	defer p.mux.Unlock()

	p.lastProcessedIndex = index
}

// Prune removes all messages whose index is less (or equal)
// to the given argument
func (p *SAMPool) Prune(index uint64) {
	p.mux.Lock()
	defer p.mux.Unlock()

	for idx := range p.messages {
		if idx <= index {
			delete(p.messages, idx)
		}
	}

	p.lastProcessedIndex = index

	p.logger.Debug("pruned stale SAMs", "last_processed_index", index)
}

// Peek returns all SAMs whose index matches the expected event.
// Messages are not removed from the pool.
func (p *SAMPool) Peek() rootchain.VerifiedSAM {
	p.mux.Lock()
	defer p.mux.Unlock()

	expectedIndex := p.lastProcessedIndex + 1

	bucket := p.messages[expectedIndex]
	if bucket == nil {
		p.logger.Debug("no SAMs for expected index", "index", expectedIndex)

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

// Pop returns all the SAMs whose index matches the expected event.
// Messages are removed from the pool
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

	p.logger.Debug(
		"added SAM",
		"index", msg.Index,
		"hash", msg.Hash.String(),
		"signature", hex.EncodeToHex(msg.Signature),
	)
}
