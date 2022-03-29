package sam

import (
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/bridge/utils"
	"github.com/0xPolygon/polygon-edge/types"
)

type pool struct {
	// write-lock is called only during the validator change process
	// otherwise read-lock is called
	// Changing validators occurs rarely (once per epoch)
	changeValidatorsLock sync.RWMutex
	validatorSet         utils.ValidatorSet
	validatorsChangeSub  utils.UpdateValidatorsSubscription

	// storage for message body
	messageMap sync.Map // types.Hash -> []byte

	// map from message Hash -> bool for message flags
	consumedMap sync.Map
	readyMap    sync.Map

	// signatures
	messageSignatures *messageSignaturesStore
}

func NewPool(validators utils.ValidatorSet) Pool {
	return &pool{
		validatorSet:      validators,
		messageSignatures: newMessageSignaturesStore(),
	}
}

func (p *pool) Start() {
	p.watchValidatorsChange()
}

func (p *pool) Close() {
	if p.validatorsChangeSub != nil {
		p.validatorSet.Unsubscribe(p.validatorsChangeSub)
		p.validatorsChangeSub = nil
	}
}

func (p *pool) watchValidatorsChange() {
	if p.validatorsChangeSub != nil {
		p.validatorSet.Unsubscribe(p.validatorsChangeSub)
	}

	p.validatorsChangeSub = p.validatorSet.Subscribe()

	p.validatorsChangeSub.Subscribe(func(ev utils.UpdateValidatorsEvent) {
		p.updateValidatorSet(ev.ValidatorDeletion, ev.OldThreshold, ev.NewThreshold)
	})
}

// Add adds new message with the signature to pool
func (p *pool) AddMessage(message *Message) {
	p.changeValidatorsLock.RLock()
	defer p.changeValidatorsLock.RUnlock()

	if p.hasConsumed(message.Hash) {
		// we do no longer put the signature if the message has been consumed
		return
	}

	p.setMessageData(message.Hash, message.Data)
	p.tryToPromote(message.Hash)
}

// Add adds new message with the signature to pool
func (p *pool) AddSignature(signature *MessageSignature) {
	p.changeValidatorsLock.RLock()
	defer p.changeValidatorsLock.RUnlock()

	if p.hasConsumed(signature.Hash) {
		// we no longer put the signature if the message has been consumed
		return
	}

	p.messageSignatures.Put(signature)
	p.tryToPromote(signature.Hash)
}

// ConsumeMessage sets the consumed flag and delete the message from pool
func (p *pool) ConsumeMessage(hash types.Hash) {
	p.consumedMap.Store(hash, true)

	p.messageSignatures.RemoveMessage(hash)
	p.messageMap.Delete(hash)
	p.readyMap.Delete(hash)
}

// IsMessageKnown returns the flag indicating the message is known
func (p *pool) IsMessageKnown(hash types.Hash) bool {
	_, ok := p.messageMap.Load(hash)

	return ok
}

// IsMessageKnown returns the flag indicating the message is known
func (p *pool) IsMessageConsumed(hash types.Hash) bool {
	raw, ok := p.consumedMap.Load(hash)

	return ok && raw.(bool)
}

func (p *pool) GetSignatureCount(hash types.Hash) uint64 {
	return p.messageSignatures.GetSignatureCount(hash)
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

func (p *pool) GetMessageByHash(hash types.Hash) ReadyMessage {
	p.changeValidatorsLock.RLock()
	defer p.changeValidatorsLock.RUnlock()

	data := p.getMessageData(hash)
	signatures := p.messageSignatures.GetSignatures(hash)

	return ReadyMessage{
		Hash:       hash,
		Data:       data,
		Signatures: signatures,
	}
}

// GetReadyMessages returns the messages with enough signatures
func (p *pool) GetReadyMessages() []ReadyMessage {
	p.changeValidatorsLock.RLock()
	defer p.changeValidatorsLock.RUnlock()

	res := make([]ReadyMessage, 0)

	p.readyMap.Range(func(key, value interface{}) bool {
		hash, _ := key.(types.Hash)

		if ready, _ := value.(bool); !ready {
			return true
		}

		data := p.getMessageData(hash)
		if data == nil {
			return true
		}

		signatures := p.messageSignatures.GetSignatures(hash)
		res = append(res, ReadyMessage{
			Data:       data,
			Hash:       hash,
			Signatures: signatures,
		})

		return true
	})

	return res
}

// UpdateValidatorSet update validators and threshold
// This process blocks other processes because messages would lose the signatures
func (p *pool) updateValidatorSet(
	validatorDeletion []types.Address,
	oldThreshold uint64,
	newThreshold uint64,
) {
	p.changeValidatorsLock.Lock()
	defer p.changeValidatorsLock.Unlock()

	var maybeDemotedHashes []types.Hash
	if len(validatorDeletion) > 0 {
		maybeDemotedHashes = p.messageSignatures.RemoveSignatures(validatorDeletion)
	}

	if oldThreshold != newThreshold {
		// we need to check all messages if threshold changes
		p.tryToPromoteAndDemoteAll()
	} else if len(maybeDemotedHashes) > 0 {
		for _, hash := range maybeDemotedHashes {
			p.tryToDemote(hash)
		}
	}
}

// canPromote return the flag indicating it's possible to change status to ready
// message need to have enough signatures and be known by pool for promotion
func (p *pool) canPromote(hash types.Hash) bool {
	isKnown := p.IsMessageKnown(hash)
	numSignatures := p.messageSignatures.GetSignatureCount(hash)
	threshold := p.validatorSet.Threshold()

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
	threshold := p.validatorSet.Threshold()

	p.messageSignatures.RangeMessages(func(entry *signedMessageEntry) bool {
		hash := entry.Hash
		isKnown := p.IsMessageKnown(hash)
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

func (p *pool) setMessageData(hash types.Hash, data interface{}) {
	p.messageMap.Store(hash, data)
}

func (p *pool) getMessageData(hash types.Hash) interface{} {
	data, existed := p.messageMap.Load(hash)
	if !existed {
		return nil
	}

	return data
}

// signedMessageEntry is representing the data stored in messageSignaturesStore
type signedMessageEntry struct {
	Hash           types.Hash
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

	return uint64(newNumSignatures)
}

// IncrementNumSignatures decrements SignatureCount and return new count
func (e *signedMessageEntry) DecrementNumSignatures() uint64 {
	newNumSignatures := atomic.AddInt64(&e.SignatureCount, -1)
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
func (m *messageSignaturesStore) GetSignatures(hash types.Hash) [][]byte {
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

	return signatures
}

// RangeMessages iterates all messages in store
func (m *messageSignaturesStore) RangeMessages(handler func(*signedMessageEntry) bool) {
	m.Range(func(_key, value interface{}) bool {
		entry, _ := value.(*signedMessageEntry)

		return handler(entry)
	})
}

// PutMessage puts new signature to one message
func (m *messageSignaturesStore) Put(signature *MessageSignature) uint64 {
	value, _ := m.LoadOrStore(signature.Hash,
		&signedMessageEntry{
			Hash:           signature.Hash,
			Signatures:     sync.Map{},
			SignatureCount: 0,
		},
	)

	entry, _ := value.(*signedMessageEntry)

	if _, loaded := entry.Signatures.LoadOrStore(signature.Address, signature.Signature); !loaded {
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
	maybeDemotedHashes := make([]types.Hash, 0)

	m.RangeMessages(func(entry *signedMessageEntry) bool {
		count := 0
		for _, addr := range addresses {
			if _, deleted := entry.Signatures.LoadAndDelete(addr); deleted {
				entry.DecrementNumSignatures()
				count++
			}
		}

		if count > 0 {
			maybeDemotedHashes = append(maybeDemotedHashes, entry.Hash)
		}

		return true
	})

	return maybeDemotedHashes
}
