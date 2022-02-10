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

	if p.hasConsumed(msg.ID) {
		// we do no longer put the signature if the message has been consumed
		return
	}

	p.messageSignatures.PutMessage(msg)
	p.tryToPromote(msg.ID)
}

// MarkAsKnown sets the known flag so that make the message promotable
func (p *pool) MarkAsKnown(id uint64) {
	p.knownMap.Store(id, true)
	p.tryToPromote(id)
}

// Consume sets the consumed flag and delete the message from pool
func (p *pool) Consume(id uint64) {
	p.consumedMap.Store(id, true)

	if p.messageSignatures.RemoveMessage(id) {
		p.readyMap.Delete(id)
		p.knownMap.Delete(id)
	}
}

// knows returns the flag indicating the message is known
func (p *pool) knows(id uint64) bool {
	raw, ok := p.knownMap.Load(id)
	if !ok {
		return false
	}

	known, ok := raw.(bool)

	return ok && known
}

// consumed returns the flag indicating the message is consumed
func (p *pool) hasConsumed(id uint64) bool {
	raw, ok := p.consumedMap.Load(id)
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
		id, _ := key.(uint64)
		ready, _ := value.(bool)

		if ready {
			if data := p.messageSignatures.GetMessage(id); data != nil {
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

	var maybeDemotableIDs []uint64
	if removed := diffAddresses(oldValidators, validators); len(removed) > 0 {
		maybeDemotableIDs = p.messageSignatures.RemoveSignatures(removed)
	}

	if oldThreshold != threshold {
		// we need to check all messages if threshold changes
		p.tryToPromoteAndDemoteAll()
	} else if len(maybeDemotableIDs) > 0 {
		for _, id := range maybeDemotableIDs {
			p.tryToDemote(id)
		}
	}
}

// canPromote return the flag indicating it's possible to change status to ready
// message need to have enough signatures and be known by pool for promotion
func (p *pool) canPromote(id uint64) bool {
	isKnown := p.knows(id)
	numSignatures := p.messageSignatures.GetSignatureCount(id)
	threshold := atomic.LoadUint64(&p.threshold)

	return isKnown && numSignatures >= threshold
}

// canDemote return the flag indicating it's possible to change status to pending
func (p *pool) canDemote(id uint64) bool {
	return !p.canPromote(id)
}

// tryToPromote checks the number of signatures and threshold and update message status to ready if need
func (p *pool) tryToPromote(id uint64) {
	if p.canPromote(id) {
		p.promote(id)
	}
}

// tryToDemote checks the number of signatures and threshold and update message status to pending if need
func (p *pool) tryToDemote(id uint64) {
	if p.canDemote(id) {
		p.demote(id)
	}
}

// tryToPromoteAndDemoteAll iterates all messages and update its statuses
func (p *pool) tryToPromoteAndDemoteAll() {
	threshold := atomic.LoadUint64(&p.threshold)

	p.messageSignatures.RangeMessages(func(entry *signedMessageEntry) bool {
		id := entry.Message.ID
		isKnown := p.knows(id)
		numSignatures := entry.NumSignatures()

		if numSignatures >= threshold && isKnown {
			p.promote(id)
		} else {
			p.demote(id)
		}

		return true
	})
}

// promote change message status to ready
func (p *pool) promote(id uint64) {
	p.readyMap.Store(id, true)
}

// promote change message status to pending
// it deletes instead of unsetting for less-complexity on getting ready messages
func (p *pool) demote(id uint64) {
	p.readyMap.Delete(id)
}

// signedMessageEntry is representing the data stored in messageSignaturesStore
type signedMessageEntry struct {
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
// messageID (uint64) -> address (types.Address) -> signature ([]byte)
type messageSignaturesStore struct {
	sync.Map
}

func newMessageSignaturesStore() *messageSignaturesStore {
	return &messageSignaturesStore{}
}

func (m *messageSignaturesStore) HasMessage(id uint64) bool {
	_, loaded := m.Load(id)

	return loaded
}

// GetSignatureCount returns the number of stored signatures for given message ID
func (m *messageSignaturesStore) GetSignatureCount(id uint64) uint64 {
	value, loaded := m.Load(id)
	if !loaded {
		return 0
	}

	entry, _ := value.(*signedMessageEntry)

	return entry.NumSignatures()
}

// GetMessage returns the message and its signatures for given message ID
func (m *messageSignaturesStore) GetMessage(id uint64) *MessageAndSignatures {
	value, loaded := m.Load(id)
	if !loaded {
		return nil
	}

	entry, _ := value.(*signedMessageEntry)
	signatures := make([][]byte, 0, entry.SignatureCount)

	entry.Signatures.Range(func(key, value interface{}) bool {
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
	m.Range(func(key, value interface{}) bool {
		entry, _ := value.(*signedMessageEntry)

		return handler(entry)
	})
}

// PutMessage puts new signature to one message
func (m *messageSignaturesStore) PutMessage(message *SignedMessage) uint64 {
	value, _ := m.LoadOrStore(message.ID,
		&signedMessageEntry{
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
func (m *messageSignaturesStore) RemoveMessage(id uint64) bool {
	_, existed := m.LoadAndDelete(id)

	return existed
}

// RemoveMessage removes the signatures by given addresses from all messages
func (m *messageSignaturesStore) RemoveSignatures(addresses []types.Address) []uint64 {
	maybeDemotableIDs := make([]uint64, 0)

	m.RangeMessages(func(entry *signedMessageEntry) bool {
		count := 0
		for _, addr := range addresses {
			if _, deleted := entry.Signatures.LoadAndDelete(addr); deleted {
				entry.DecrementNumSignatures()
				count++
			}
		}

		if count > 0 {
			maybeDemotableIDs = append(maybeDemotableIDs, entry.Message.ID)
		}

		return true
	})

	return maybeDemotableIDs
}
