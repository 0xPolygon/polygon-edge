package slashing

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"sync"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/common"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	errViewUndefined           = errors.New("view is undefined")
	errInvalidMsgType          = errors.New("message type is invalid")
	errUnknownSender           = errors.New("message sender is not a known validator")
	errSignerAndSenderMismatch = errors.New("signer and sender of IBFT message are not the same")
)

type MinAddressHeap []types.Address

func (h MinAddressHeap) Len() int           { return len(h) }
func (h MinAddressHeap) Less(i, j int) bool { return bytes.Compare(h[i].Bytes(), h[j].Bytes()) < 0 }
func (h MinAddressHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *MinAddressHeap) Push(x interface{}) {
	*h = append(*h, x.(types.Address)) //nolint:forcetypeassert
}

func (h *MinAddressHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]

	return x
}

type MinRoundsHeap []uint64

func (h MinRoundsHeap) Len() int           { return len(h) }
func (h MinRoundsHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h MinRoundsHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *MinRoundsHeap) Push(x interface{}) {
	*h = append(*h, x.(uint64)) //nolint:forcetypeassert
}

func (h *MinRoundsHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]

	return x
}

var _ sort.Interface = (*SortedMessages)(nil)

type SortedMessages []*ibftProto.Message

func (s SortedMessages) Len() int           { return len(s) }
func (s SortedMessages) Less(i, j int) bool { return bytes.Compare(s[i].Signature, s[j].Signature) < 0 }
func (s SortedMessages) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (s *SortedMessages) Add(msg *ibftProto.Message) {
	index := sort.Search(len(*s), func(i int) bool {
		return bytes.Compare((*s)[i].Signature, msg.Signature) >= 0
	})

	*s = append(*s, &ibftProto.Message{})
	copy((*s)[index+1:], (*s)[index:])
	(*s)[index] = msg
}

type SenderMessagesMap map[types.Address]SortedMessages
type MessagesMap map[uint64]map[uint64]SenderMessagesMap

type DoubleSigners []types.Address

// containsSender checks whether provided sender is already present in the double sign evidences
func (s *DoubleSigners) containsSender(sender types.Address) bool {
	for _, addr := range *s {
		if addr == sender {
			return true
		}
	}

	return false
}

//nolint:godox
// TODO: RLP serialize/deserialize methods, Equals method for validation, etc.

type DoubleSignEvidence struct {
	sender   types.Address
	messages []*ibftProto.Message
}

func newDoubleSignEvidence(sender types.Address, messages []*ibftProto.Message) *DoubleSignEvidence {
	return &DoubleSignEvidence{sender: sender, messages: messages}
}

type Messages struct {
	content       MessagesMap
	sortedSenders map[uint64]MinAddressHeap
	sortedRounds  map[uint64]MinRoundsHeap
	mux           sync.RWMutex
}

// getSenderMsgs returns messages for given height, round and sender.
func (m *Messages) getSenderMsgs(view *ibftProto.View, sender types.Address) []*ibftProto.Message {
	m.mux.RLock()
	defer m.mux.RUnlock()

	if roundMsgs, ok := m.content[view.Height]; ok {
		if senderMsgsMap, ok := roundMsgs[view.Round]; ok {
			return senderMsgsMap[sender]
		}
	}

	return nil
}

// addMessage registers message for the given height, round and sender
func (m *Messages) addMessage(view *ibftProto.View, sender types.Address, msg *ibftProto.Message) {
	m.mux.Lock()
	defer m.mux.Unlock()

	// add message
	var senderMsgs SenderMessagesMap

	if roundMsgs, ok := m.content[view.Height]; !ok {
		roundMsgs = make(map[uint64]SenderMessagesMap)
		m.content[view.Height] = roundMsgs
		senderMsgs = createSenderMsgsMap(view.Round, roundMsgs)
	} else if senderMsgs, ok = roundMsgs[view.Round]; !ok {
		senderMsgs = createSenderMsgsMap(view.Round, roundMsgs)
	}

	msgs, ok := senderMsgs[sender]
	if !ok {
		msgs = make([]*ibftProto.Message, 0, 1)
	}

	msgs.Add(msg)
	senderMsgs[sender] = msgs

	// add sender into sorted sender heap
	sendersHeap, ok := m.sortedSenders[view.Height]
	if !ok {
		sendersHeap = make(MinAddressHeap, 0, 1)
	}

	sendersHeap.Push(sender)
	m.sortedSenders[view.Height] = sendersHeap

	// add round into sorted rounds heap
	roundsHeap, ok := m.sortedRounds[view.Height]
	if !ok {
		roundsHeap = make(MinRoundsHeap, 0, 1)
	}

	roundsHeap.Push(view.Round)
	m.sortedRounds[view.Height] = roundsHeap
}

// createSenderMsgsMap initializes senders message map for the given round
func createSenderMsgsMap(round uint64, roundMsgs map[uint64]SenderMessagesMap) SenderMessagesMap {
	sendersMsgs := make(SenderMessagesMap)
	roundMsgs[round] = sendersMsgs

	return sendersMsgs
}

var _ DoubleSigningTracker = (*DoubleSigningTrackerImpl)(nil)

// DoubleSigningTracker is an abstraction for gathering IBFT messages,
// storing them and providing double signing evidences
type DoubleSigningTracker interface {
	Handle(msg *ibftProto.Message)
	GetDoubleSigners(height uint64) DoubleSigners
	PruneMsgsUntil(height uint64)
	PostBlock(req *common.PostBlockRequest) error
}

type DoubleSigningTrackerImpl struct {
	preprepare  *Messages
	prepare     *Messages
	commit      *Messages
	roundChange *Messages

	msgsTypes          []ibftProto.MessageType
	mux                sync.RWMutex
	validatorsProvider validator.ValidatorsProvider
	validators         validator.AccountSet
	logger             hclog.Logger
}

func NewDoubleSigningTracker(logger hclog.Logger,
	validatorsProvider validator.ValidatorsProvider) (*DoubleSigningTrackerImpl, error) {
	initialValidators, err := validatorsProvider.GetAllValidators()
	if err != nil {
		return nil, err
	}

	t := &DoubleSigningTrackerImpl{
		logger:             logger,
		validatorsProvider: validatorsProvider,
		validators:         initialValidators,
		preprepare: &Messages{
			content:       make(MessagesMap),
			sortedSenders: make(map[uint64]MinAddressHeap),
			sortedRounds:  make(map[uint64]MinRoundsHeap),
		},
		prepare: &Messages{
			content:       make(MessagesMap),
			sortedSenders: make(map[uint64]MinAddressHeap),
			sortedRounds:  make(map[uint64]MinRoundsHeap),
		},
		commit: &Messages{
			content:       make(MessagesMap),
			sortedSenders: make(map[uint64]MinAddressHeap),
			sortedRounds:  make(map[uint64]MinRoundsHeap),
		},
		roundChange: &Messages{
			content:       make(MessagesMap),
			sortedSenders: make(map[uint64]MinAddressHeap),
			sortedRounds:  make(map[uint64]MinRoundsHeap),
		},
	}

	for _, msgType := range ibftProto.MessageType_value {
		t.msgsTypes = append(t.msgsTypes, ibftProto.MessageType(msgType))
	}

	sort.Slice(t.msgsTypes, func(i, j int) bool {
		return t.msgsTypes[i] < t.msgsTypes[j]
	})

	return t, nil
}

// Handle is implementation of IBFTMessageHandler interface, which handles IBFT consensus messages
func (t *DoubleSigningTrackerImpl) Handle(msg *ibftProto.Message) {
	if err := t.validateMsg(msg); err != nil {
		t.logger.Debug("[ERROR] invalid IBFT message retrieved, ignoring it.", "error", err, "message", msg)

		return
	}

	t.mux.Lock()
	defer t.mux.Unlock()

	msgMap := t.resolveMessagesStorage(msg.GetType())
	msgMap.addMessage(msg.View, types.BytesToAddress(msg.From), msg)
}

// PruneMsgsUntil deletes all messages maps until the specified height
func (t *DoubleSigningTrackerImpl) PruneMsgsUntil(height uint64) {
	for _, msgType := range t.msgsTypes {
		msgs := t.resolveMessagesStorage(msgType)
		if msgs == nil {
			continue
		}

		msgs.mux.Lock()

		for msgHeight := range msgs.content {
			if msgHeight < height {
				delete(msgs.content, msgHeight)
			}
		}

		for sendersHeight := range msgs.sortedSenders {
			if sendersHeight < height {
				delete(msgs.sortedSenders, sendersHeight)
			}
		}

		for roundsHeight := range msgs.sortedRounds {
			if roundsHeight < height {
				delete(msgs.sortedRounds, roundsHeight)
			}
		}

		msgs.mux.Unlock()
	}
}

// GetEvidences returns double signers for the given height
func (t *DoubleSigningTrackerImpl) GetDoubleSigners(height uint64) DoubleSigners {
	doubleSigners := make(DoubleSigners, 0)

	for _, msgType := range t.msgsTypes {
		msgs := t.resolveMessagesStorage(msgType)
		if msgs == nil {
			continue
		}

		msgs.mux.Lock()

		roundMsgs, msgsMapExists := msgs.content[height]
		sendersHeap, sendersHeapExists := msgs.sortedSenders[height]
		roundsHeap, roundsHeapExists := msgs.sortedRounds[height]

		if !msgsMapExists || !sendersHeapExists || !roundsHeapExists {
			continue
		}

		for roundsHeap.Len() > 0 {
			round := roundsHeap.Pop().(uint64) //nolint:forcetypeassert
			msgsPerSenders := roundMsgs[round]

			for sendersHeap.Len() > 0 {
				sender := sendersHeap.Pop().(types.Address) //nolint:forcetypeassert

				msgs, ok := msgsPerSenders[sender]
				if !ok || len(msgs) < 2 || doubleSigners.containsSender(sender) {
					continue
				}

				doubleSigners = append(doubleSigners, sender)
			}
		}

		msgs.mux.Unlock()
	}

	return doubleSigners
}

// PostBlock is used to populate all known validators
func (t *DoubleSigningTrackerImpl) PostBlock(_ *common.PostBlockRequest) error {
	validators, err := t.validatorsProvider.GetAllValidators()
	if err != nil {
		return err
	}

	t.mux.Lock()
	defer t.mux.Unlock()

	t.validators = validators

	return nil
}

// validateMsg validates provided IBFT message
func (t *DoubleSigningTrackerImpl) validateMsg(msg *ibftProto.Message) error {
	// check is view defined
	if msg.View == nil {
		return errViewUndefined
	}

	// check message type
	if _, ok := ibftProto.MessageType_name[int32(msg.Type)]; !ok {
		return errInvalidMsgType
	}

	// recover message signer
	signer, err := wallet.RecoverSignerFromIBFTMessage(msg)
	if err != nil {
		return err
	}

	sender := types.BytesToAddress(msg.From)

	t.mux.RLock()
	defer t.mux.RUnlock()

	// ignore messages which originate from unknown validator
	if !t.validators.ContainsAddress(sender) {
		return errUnknownSender
	}

	// ignore messages where signer and sender are not the same
	if signer != sender {
		return errSignerAndSenderMismatch
	}

	msgsMap := t.resolveMessagesStorage(msg.Type)

	// ignore messages which are already present in the storage in order to prevent DDOS attack
	senderMsgs := msgsMap.getSenderMsgs(msg.View, sender)
	for _, senderMsg := range senderMsgs {
		if bytes.Equal(senderMsg.Signature, msg.Signature) {
			return fmt.Errorf("sender %s is detected as a spammer", sender)
		}
	}

	return nil
}

// resolveMessagesStorage resolves proper messages storage based on the provided message type
func (t *DoubleSigningTrackerImpl) resolveMessagesStorage(msgType ibftProto.MessageType) *Messages {
	switch msgType {
	case ibftProto.MessageType_PREPREPARE:
		return t.preprepare
	case ibftProto.MessageType_PREPARE:
		return t.prepare
	case ibftProto.MessageType_COMMIT:
		return t.commit
	case ibftProto.MessageType_ROUND_CHANGE:
		return t.roundChange
	}

	return nil
}
