package sam

import (
	"sync"

	"github.com/0xPolygon/polygon-edge/types"
)

type Message struct {
	ID      uint64
	Payload []byte
}

type MessageAndSignatures struct {
	Message    *Message
	Signatures map[types.Address][]byte
}

type Signer interface {
	Sign([]byte) ([]byte, error)
	Address() types.Address
}

type SignatureRecoverer interface {
	Recover([]byte) (types.Address, error)
}

type Pool interface {
	Start()
	Close()
	SetValidators([]types.Address, uint64)
	AddMessage(*Message) error
	AddSignedMessage(*Message, []byte) error
	TakeEnoughSignedMessages(*uint64) []*MessageAndSignatures
}

type pool struct {
	signer    Signer
	recoverer SignatureRecoverer

	validatorsLock sync.RWMutex
	validators     []types.Address
	threshold      uint64 // required number of signatures for ready

	pendingMap *messageMap // for messages with not-enough signatures
	readyMap   *messageMap // for messages with enough signatures

	removeSignaturesJobQueueLock sync.Mutex
	removeSignaturesJobQueue     []*removeSignaturesJob

	newRemoveSignaturesJobSignal chan struct{}
	closeCh                      chan struct{}
}

type removeSignaturesJob struct {
	Removed []types.Address
}

func diffAddresses(arr1, arr2 []types.Address) []types.Address {
	arr2Map := make(map[types.Address]bool)
	for _, addr := range arr2 {
		arr2Map[addr] = true
	}

	diff := make([]types.Address, 0)

	for _, addr := range arr2 {
		if !arr2Map[addr] {
			diff = append(diff, addr)
		}
	}

	return diff
}

func NewPool(signer Signer, recoverer SignatureRecoverer, validators []types.Address, threshold uint64) Pool {
	return &pool{
		signer:                       signer,
		recoverer:                    recoverer,
		validatorsLock:               sync.RWMutex{},
		validators:                   validators,
		threshold:                    threshold,
		pendingMap:                   newMessageMap(),
		readyMap:                     newMessageMap(),
		removeSignaturesJobQueueLock: sync.Mutex{},
		removeSignaturesJobQueue:     make([]*removeSignaturesJob, 0),
		newRemoveSignaturesJobSignal: make(chan struct{}, 1),
		closeCh:                      make(chan struct{}, 1),
	}
}

func (p *pool) Start() {
	go p.processEvents()
}

func (p *pool) Close() {
	select {
	case p.closeCh <- struct{}{}:
	default:
	}
}

func (p *pool) processEvents() {
	for {
		select {
		case <-p.newRemoveSignaturesJobSignal:
			p.processRemoveValidatorsJobs()
		case <-p.closeCh:
			return
		}
	}
}

func (p *pool) SetValidators(validators []types.Address, threshold uint64) {
	p.validatorsLock.Lock()
	defer p.validatorsLock.Unlock()

	oldValidators := p.validators

	p.validators = validators
	p.threshold = threshold

	if removed := diffAddresses(oldValidators, validators); len(removed) > 0 {
		p.removeSignaturesJobQueueLock.Lock()
		p.removeSignaturesJobQueue = append(p.removeSignaturesJobQueue, &removeSignaturesJob{
			Removed: removed,
		})
		p.removeSignaturesJobQueueLock.Unlock()
	}
}

func (p *pool) AddMessage(msg *Message) error {
	signature, err := p.signer.Sign(msg.Payload)
	if err != nil {
		return err
	}

	p.addMessage(msg, p.signer.Address(), signature)

	return nil
}

func (p *pool) AddSignedMessage(msg *Message, signature []byte) error {
	addr, err := p.recoverer.Recover(signature)
	if err != nil {
		return err
	}

	p.addMessage(msg, addr, signature)

	return nil
}

func (p *pool) TakeEnoughSignedMessages(num *uint64) []*MessageAndSignatures {
	return p.readyMap.RemoveEnoughSignedMessages(p.threshold, num)
}

func (p *pool) addMessage(msg *Message, address types.Address, signature []byte) {
	data := &MessageAndSignatures{
		Message: msg,
		Signatures: map[types.Address][]byte{
			address: signature,
		},
	}

	if p.readyMap.Has(msg.ID) {
		p.readyMap.Put(data)

		return
	}

	// XXX: race condition?
	numSigs := p.pendingMap.Put(data)
	if numSigs >= p.threshold {
		msg := p.pendingMap.RemoveByID(msg.ID)
		p.readyMap.Put(msg)
	}
}

func (p *pool) processRemoveValidatorsJobs() {
	for {
		p.removeSignaturesJobQueueLock.Lock()
		if len(p.removeSignaturesJobQueue) == 0 {
			p.removeSignaturesJobQueueLock.Unlock()

			return
		}

		var job *removeSignaturesJob
		job, p.removeSignaturesJobQueue = p.removeSignaturesJobQueue[0], p.removeSignaturesJobQueue[1:]

		p.removeSignaturesJobQueueLock.Unlock()
		p.processRemoveValidatorsJob(job)
	}
}

func (p *pool) processRemoveValidatorsJob(job *removeSignaturesJob) {
	p.readyMap.DeleteSignatures(job.Removed)
	p.pendingMap.DeleteSignatures(job.Removed)

	// push back the messages with not enough signatures to pending map
	msgs := p.readyMap.RemoveNotEnoughSignedMessages(p.threshold, nil)
	for _, msg := range msgs {
		p.pendingMap.Put(msg)
	}
}

type messageRecord struct {
	message        *Message
	signatures     map[types.Address][]byte
	signaturesLock *sync.RWMutex
}

// messageMap is a map from uint64 ID to messageRecord
type messageMap struct {
	sync.Map
}

func newMessageMap() *messageMap {
	return &messageMap{}
}

func (m *messageMap) Has(id uint64) bool {
	_, loaded := m.Load(id)

	return loaded
}

func (m *messageMap) Get(id uint64) *MessageAndSignatures {
	value, loaded := m.Load(id)
	if !loaded {
		return nil
	}

	msg, _ := value.(*messageRecord)

	return &MessageAndSignatures{
		Message:    msg.message,
		Signatures: msg.signatures,
	}
}

func (m *messageMap) Put(data *MessageAndSignatures) uint64 {
	value, loaded := m.LoadOrStore(data.Message.ID,
		// default record
		&messageRecord{
			message:        data.Message,
			signatures:     data.Signatures,
			signaturesLock: &sync.RWMutex{},
		},
	)

	if !loaded {
		return uint64(len(data.Signatures))
	}

	record, _ := value.(*messageRecord)
	record.signaturesLock.Lock()
	defer record.signaturesLock.Unlock()

	for addr, sig := range data.Signatures {
		record.signatures[addr] = sig
	}

	return uint64(len(record.signatures))
}

func (m *messageMap) DeleteSignatures(addresses []types.Address) {
	m.Range(func(_, value interface{}) bool {
		record, _ := value.(*messageRecord)

		record.signaturesLock.Lock()
		for _, addr := range addresses {
			delete(record.signatures, addr)
		}

		record.signaturesLock.Unlock()

		return true
	})
}

func (m *messageMap) RemoveByID(id uint64) *MessageAndSignatures {
	value, loaded := m.LoadAndDelete(id)
	if !loaded {
		return nil
	}

	record, _ := value.(*messageRecord)

	return &MessageAndSignatures{
		Message:    record.message,
		Signatures: record.signatures,
	}
}

func (m *messageMap) RemoveEnoughSignedMessages(threshold uint64, num *uint64) []*MessageAndSignatures {
	return m.removeBy(func(r *messageRecord) bool {
		return uint64(len(r.signatures)) >= threshold
	}, num)
}

func (m *messageMap) RemoveNotEnoughSignedMessages(threshold uint64, num *uint64) []*MessageAndSignatures {
	return m.removeBy(func(r *messageRecord) bool {
		return uint64(len(r.signatures)) < threshold
	}, num)
}

func (m *messageMap) removeBy(isValid func(*messageRecord) bool, num *uint64) []*MessageAndSignatures {
	msgs := []*MessageAndSignatures{}

	m.Range(func(key, value interface{}) bool {
		record, _ := value.(*messageRecord)

		record.signaturesLock.RLock()
		valid := isValid(record)

		record.signaturesLock.RUnlock()

		if !valid {
			return true
		}

		m.Delete(key)
		msgs = append(msgs, &MessageAndSignatures{
			Message:    record.message,
			Signatures: record.signatures,
		})

		return num == nil || uint64(len(msgs)) < *num
	})

	return msgs
}
