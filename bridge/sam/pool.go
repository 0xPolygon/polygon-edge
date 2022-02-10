package sam

import (
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/types"
)

type pool struct {
	// Lock is called only when changing validators process
	// otherwise Rlock is called
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

func (p *pool) Add(msg *SignedMessage) {
	p.changeValidatorsLock.RLock()
	defer p.changeValidatorsLock.RUnlock()

	consumed, ok := p.consumedMap.Load(msg.ID)
	if ok && consumed.(bool) {
		return
	}

	p.messageSignatures.PutMessage(msg)
	p.tryToPromote(msg.ID)
}

func (p *pool) MarkAsKnown(id uint64) {
	p.knownMap.Store(id, true)
	p.tryToPromote(id)
}

func (p *pool) Consume(id uint64) {
	p.consumedMap.Store(id, true)

	if p.messageSignatures.RemoveMessage(id) {
		p.readyMap.Delete(id)
		p.knownMap.Delete(id)
	}
}

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

func (p *pool) UpdateValidatorSet(validators []types.Address, threshold uint64) {
	p.changeValidatorsLock.Lock()
	defer p.changeValidatorsLock.Unlock()

	oldValidators := p.validators
	oldThreshold := p.threshold //nolint

	p.validators = validators

	atomic.StoreUint64(&p.threshold, threshold)

	removed := diffAddresses(oldValidators, validators)

	var demotableIDs []uint64
	if len(removed) > 0 {
		demotableIDs = p.messageSignatures.RemoveSignatures(removed)
	}

	if oldThreshold != threshold {
		p.tryToPromoteAndDemoteAll()
	} else if len(demotableIDs) > 0 {
		p.tryToDemote(demotableIDs)
	}
}

func (p *pool) canPromote(id uint64) bool {
	isKnown, ok := p.knownMap.Load(id)
	numSignatures := p.messageSignatures.GetSignatureCount(id)
	threshold := atomic.LoadUint64(&p.threshold)

	return ok && isKnown.(bool) && numSignatures >= threshold
}

func (p *pool) canDemote(id uint64) bool {
	isReady, ok := p.readyMap.Load(id)

	return !p.canPromote(id) && ok && isReady.(bool)
}

func (p *pool) tryToPromote(id uint64) {
	if p.canPromote(id) {
		p.promote(id)
	}
}

func (p *pool) tryToDemote(ids []uint64) {
	for _, id := range ids {
		if p.canDemote(id) {
			p.demote(id)
		}
	}
}

func (p *pool) tryToPromoteAndDemoteAll() {
	threshold := atomic.LoadUint64(&p.threshold)

	p.messageSignatures.RangeMessages(func(entry *signedMessageEntry) bool {
		id := entry.Message.ID

		isKnown, ok := p.knownMap.Load(id)
		numSignatures := entry.NumSignatures()
		isReady := numSignatures >= threshold && ok && isKnown.(bool)

		if isReady {
			p.promote(id)
		} else {
			p.demote(id)
		}

		return true
	})
}

func (p *pool) promote(id uint64) {
	p.readyMap.Store(id, true)
}

func (p *pool) demote(id uint64) {
	p.readyMap.Store(id, false)
}

// warning: SignatureCount optimistic concurrency
type signedMessageEntry struct {
	Message        Message
	Signatures     sync.Map
	SignatureCount int64
}

func (e *signedMessageEntry) NumSignatures() uint64 {
	count := atomic.LoadInt64(&e.SignatureCount)
	if count < 0 {
		return 0
	}

	return uint64(count)
}

func (e *signedMessageEntry) IncrementNumSignatures() uint64 {
	newNumSignatures := atomic.AddInt64(&e.SignatureCount, 1)
	if newNumSignatures < 0 {
		return 0
	}

	return uint64(newNumSignatures)
}

func (e *signedMessageEntry) DecrementNumSignatures() uint64 {
	newNumSignatures := atomic.AddInt64(&e.SignatureCount, ^int64(0))
	if newNumSignatures < 0 {
		return 0
	}

	return uint64(newNumSignatures)
}

// messageSignaturesStore is a nested map from ID to signatures
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

func (m *messageSignaturesStore) GetSignatureCount(id uint64) uint64 {
	value, loaded := m.Load(id)
	if !loaded {
		return 0
	}

	entry, _ := value.(*signedMessageEntry)

	return entry.NumSignatures()
}

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

func (m *messageSignaturesStore) RangeMessages(handler func(*signedMessageEntry) bool) {
	m.Range(func(key, value interface{}) bool {
		entry, _ := value.(*signedMessageEntry)

		return handler(entry)
	})
}

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

func (m *messageSignaturesStore) RemoveMessage(id uint64) bool {
	_, existed := m.LoadAndDelete(id)

	return existed
}

func (m *messageSignaturesStore) RemoveSignatures(addresses []types.Address) []uint64 {
	demotableIDs := make([]uint64, 0)

	m.RangeMessages(func(entry *signedMessageEntry) bool {
		count := 0
		for _, addr := range addresses {
			if _, deleted := entry.Signatures.LoadAndDelete(addr); deleted {
				entry.DecrementNumSignatures()
				count++
			}
		}

		if count > 0 {
			demotableIDs = append(demotableIDs, entry.Message.ID)
		}

		return true
	})

	return demotableIDs
}
