package sam

import (
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/types"
)

type pool struct {
	// write-lock is called only when changing validators process
	// otherwise read-lock is called
	// Changing validators will occur the most rarely (once per epoch)
	changeValidatorsLock sync.RWMutex
	validators           []types.Address
	threshold            uint64 // required number of signatures for ready

	// map from message ID (uint64) -> bool for message flags
	knownMap    sync.Map
	consumedMap sync.Map
	readyMap    sync.Map

	// signatures
	messageSignatures *messageSignaturesStore
}

// diffAddresses returns a list of the addresses that are in arr1 but not in arr2
func diffAddresses(arr1, arr2 []types.Address) []types.Address {
	arr2Map := make(map[types.Address]bool)
	for _, addr := range arr2 {
		arr2Map[addr] = true
	}

	diff := make([]types.Address, 0)

	for _, addr := range arr1 {
		if !arr2Map[addr] {
			diff = append(diff, addr)
		}
	}

	return diff
}

func NewPool(validators []types.Address, threshold uint64) Pool {
	return &pool{
		changeValidatorsLock: sync.RWMutex{},
		validators:           validators,
		threshold:            threshold,
		knownMap:             sync.Map{},
		consumedMap:          sync.Map{},
		readyMap:             sync.Map{},
		messageSignatures:    newMessageSignaturesStore(),
	}
}

// Add adds new message with the signature to pool
func (p *pool) Add(msg *SignedMessage) {
	p.changeValidatorsLock.RLock()
	defer p.changeValidatorsLock.RUnlock()

	if p.hasConsumed(msg.Hash) {
		// we do no longer put the signature if the message has been consumed
		return
	}

	p.messageSignatures.PutMessage(msg)
	p.tryToPromote(msg.Hash)
}

// MarkAsKnown sets the known flag so that make the message promotable
func (p *pool) MarkAsKnown(hash types.Hash) {
	p.knownMap.Store(hash, true)
	p.tryToPromote(hash)
}

// Consume sets the consumed flag and delete the message from pool
func (p *pool) Consume(hash types.Hash) {
	p.consumedMap.Store(hash, true)

	if p.messageSignatures.RemoveMessage(hash) {
		p.readyMap.Delete(hash)
		p.knownMap.Delete(hash)
	}
}

// knows returns the flag indicating the message is known
func (p *pool) knows(hash types.Hash) bool {
	raw, ok := p.knownMap.Load(hash)
	if !ok {
		return false
	}

	known, ok := raw.(bool)

	return ok && known
}

// consumed returns the flag indicating the message is consumed
func (p *pool) hasConsumed(hash types.Hash) bool {
	raw, ok := p.consumedMap.Load(hash)
	if !ok {
		return false
	}

	consumed, ok := raw.(bool)

	return ok && consumed
}

// GetReadyMessages returns the messages with enough signatures
func (p *pool) GetReadyMessages() []MessageAndSignatures {
	p.changeValidatorsLock.RLock()
	defer p.changeValidatorsLock.RUnlock()

	res := make([]MessageAndSignatures, 0)

	p.readyMap.Range(func(key, value interface{}) bool {
		hash, _ := key.(types.Hash)
		ready, _ := value.(bool)

		if ready {
			if data := p.messageSignatures.GetMessage(hash); data != nil {
				res = append(res, *data)
			}
		}

		return true
	})

	return res
}

// UpdateValidatorSet update validators and threshold
// This process blocks other processes because messages would lose the signatures
func (p *pool) UpdateValidatorSet(validators []types.Address, threshold uint64) {
	p.changeValidatorsLock.Lock()
	defer p.changeValidatorsLock.Unlock()

	oldValidators := p.validators
	oldThreshold := p.threshold //nolint

	p.validators = validators
	atomic.StoreUint64(&p.threshold, threshold)

	var maybeDemotableHashes []types.Hash
	if removed := diffAddresses(oldValidators, validators); len(removed) > 0 {
		maybeDemotableHashes = p.messageSignatures.RemoveSignatures(removed)
	}

	if oldThreshold != threshold {
		// we need to check all messages if threshold changes
		p.tryToPromoteAndDemoteAll()
	} else if len(maybeDemotableHashes) > 0 {
		for _, hash := range maybeDemotableHashes {
			p.tryToDemote(hash)
		}
	}
}

// canPromote return the flag indicating it's possible to change status to ready
// message need to have enough signatures and be known by pool for promotion
func (p *pool) canPromote(hash types.Hash) bool {
	isKnown := p.knows(hash)
	numSignatures := p.messageSignatures.GetSignatureCount(hash)
	threshold := atomic.LoadUint64(&p.threshold)

	return isKnown && numSignatures >= threshold
}

// canDemote return the flag indicating it's possible to change status to pending
func (p *pool) canDemote(hash types.Hash) bool {
	return !p.canPromote(hash)
}

// tryToPromote checks the number of signatures and threshold and update message status to ready if need
func (p *pool) tryToPromote(hash types.Hash) {
	if p.canPromote(hash) {
		p.promote(hash)
	}
}

// tryToDemote checks the number of signatures and threshold and update message status to pending if need
func (p *pool) tryToDemote(hash types.Hash) {
	if p.canDemote(hash) {
		p.demote(hash)
	}
}

// tryToPromoteAndDemoteAll iterates all messages and update its statuses
func (p *pool) tryToPromoteAndDemoteAll() {
	threshold := atomic.LoadUint64(&p.threshold)

	p.messageSignatures.RangeMessages(func(entry *signedMessageEntry) bool {
		hash := entry.Hash
		isKnown := p.knows(hash)
		numSignatures := entry.NumSignatures()

		if numSignatures >= threshold && isKnown {
			p.promote(hash)
		} else {
			p.demote(hash)
		}

		return true
	})
}

// promote change message status to ready
func (p *pool) promote(hash types.Hash) {
	p.readyMap.Store(hash, true)
}

// promote change message status to pending
// it deletes instead of unsetting for less-complexity on getting ready messages
func (p *pool) demote(hash types.Hash) {
	p.readyMap.Delete(hash)
}

// signedMessageEntry is representing the data stored in messageSignaturesStore
type signedMessageEntry struct {
	Hash           types.Hash
	Message        Message
	Signatures     sync.Map
	SignatureCount int64
}

// NumSignatures returns number of signatures
func (e *signedMessageEntry) NumSignatures() uint64 {
	count := atomic.LoadInt64(&e.SignatureCount)
	if count < 0 {
		return 0
	}

	return uint64(count)
}

// IncrementNumSignatures increments SignatureCount and return new count
func (e *signedMessageEntry) IncrementNumSignatures() uint64 {
	newNumSignatures := atomic.AddInt64(&e.SignatureCount, 1)
	if newNumSignatures < 0 {
		return 0
	}

	return uint64(newNumSignatures)
}

// IncrementNumSignatures decrements SignatureCount and return new count
func (e *signedMessageEntry) DecrementNumSignatures() uint64 {
	newNumSignatures := atomic.AddInt64(&e.SignatureCount, ^int64(0))
	if newNumSignatures < 0 {
		return 0
	}

	return uint64(newNumSignatures)
}

// messageSignaturesStore is a nested map from message ID to signatures
// messageID (types.Hash) -> address (types.Address) -> signature ([]byte)
type messageSignaturesStore struct {
	sync.Map
}

func newMessageSignaturesStore() *messageSignaturesStore {
	return &messageSignaturesStore{}
}

func (m *messageSignaturesStore) HasMessage(hash types.Hash) bool {
	_, loaded := m.Load(hash)

	return loaded
}

// GetSignatureCount returns the number of stored signatures for given message ID
func (m *messageSignaturesStore) GetSignatureCount(hash types.Hash) uint64 {
	value, loaded := m.Load(hash)
	if !loaded {
		return 0
	}

	entry, _ := value.(*signedMessageEntry)

	return entry.NumSignatures()
}

// GetMessage returns the message and its signatures for given message ID
func (m *messageSignaturesStore) GetMessage(hash types.Hash) *MessageAndSignatures {
	value, loaded := m.Load(hash)
	if !loaded {
		return nil
	}

	entry, _ := value.(*signedMessageEntry)
	signatures := make([][]byte, 0, entry.SignatureCount)

	entry.Signatures.Range(func(_key, value interface{}) bool {
		signature, _ := value.([]byte)
		signatures = append(signatures, signature)

		return true
	})

	return &MessageAndSignatures{
		Message:    &entry.Message,
		Signatures: signatures,
	}
}

// RangeMessages iterates all messages in store
func (m *messageSignaturesStore) RangeMessages(handler func(*signedMessageEntry) bool) {
	m.Range(func(_key, value interface{}) bool {
		entry, _ := value.(*signedMessageEntry)

		return handler(entry)
	})
}

// PutMessage puts new signature to one message
func (m *messageSignaturesStore) PutMessage(message *SignedMessage) uint64 {
	value, _ := m.LoadOrStore(message.Hash,
		&signedMessageEntry{
			Hash:           message.Hash,
			Message:        message.Message,
			Signatures:     sync.Map{},
			SignatureCount: 0,
		},
	)

	entry, _ := value.(*signedMessageEntry)

	if _, loaded := entry.Signatures.LoadOrStore(message.Address, message.Signature); !loaded {
		return entry.IncrementNumSignatures()
	}

	return entry.NumSignatures()
}

// RemoveMessage removes the message from store
func (m *messageSignaturesStore) RemoveMessage(hash types.Hash) bool {
	_, existed := m.LoadAndDelete(hash)

	return existed
}

// RemoveMessage removes the signatures by given addresses from all messages
func (m *messageSignaturesStore) RemoveSignatures(addresses []types.Address) []types.Hash {
	maybeDemotableHashes := make([]types.Hash, 0)

	m.RangeMessages(func(entry *signedMessageEntry) bool {
		count := 0
		for _, addr := range addresses {
			if _, deleted := entry.Signatures.LoadAndDelete(addr); deleted {
				entry.DecrementNumSignatures()
				count++
			}
		}

		if count > 0 {
			maybeDemotableHashes = append(maybeDemotableHashes, entry.Hash)
		}

		return true
	})

	return maybeDemotableHashes
}
