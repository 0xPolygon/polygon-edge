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

type SenderMessagesMap map[types.Address][]*ibftProto.Message
type MessagesMap map[uint64]map[uint64]SenderMessagesMap

type DoubleSignEvidence struct {
	signer   types.Address
	round    uint64
	messages []*ibftProto.Message
}

func newDoubleSignEvidence(signer types.Address, round uint64,
	messages []*ibftProto.Message) *DoubleSignEvidence {
	return &DoubleSignEvidence{signer: signer, round: round, messages: messages}
}

type Messages struct {
	content MessagesMap
	mux     sync.RWMutex
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

	var senderMsgs SenderMessagesMap

	if roundMsgs, ok := m.content[view.Height]; !ok {
		roundMsgs = make(map[uint64]SenderMessagesMap)
		m.content[view.Height] = roundMsgs
		senderMsgs = createSenderMsgsMap(view.Round, roundMsgs)
	} else if senderMsgs, ok = roundMsgs[view.Round]; !ok {
		senderMsgs = createSenderMsgsMap(view.Round, roundMsgs)
	}

	if _, ok := senderMsgs[sender]; ok {
		senderMsgs[sender] = append(senderMsgs[sender], msg)
	} else {
		senderMsgs[sender] = []*ibftProto.Message{msg}
	}
}

// createSenderMsgsMap initializes senders message map for the given round
func createSenderMsgsMap(round uint64, roundMsgs map[uint64]SenderMessagesMap) SenderMessagesMap {
	sendersMsgs := make(SenderMessagesMap)
	roundMsgs[round] = sendersMsgs

	return sendersMsgs
}

var _ DoubleSigningTracker = (*DoubleSigningTrackerImpl)(nil)

type DoubleSigningTracker interface {
	Handle(msg *ibftProto.Message)
	GetEvidences(height uint64) []*DoubleSignEvidence
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
	initialValidators, err := validatorsProvider.GetValidators()
	if err != nil {
		return nil, err
	}

	t := &DoubleSigningTrackerImpl{
		logger:             logger,
		validatorsProvider: validatorsProvider,
		validators:         initialValidators,
		preprepare:         &Messages{content: make(MessagesMap)},
		prepare:            &Messages{content: make(MessagesMap)},
		commit:             &Messages{content: make(MessagesMap)},
		roundChange:        &Messages{content: make(MessagesMap)},
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
		t.logger.Debug("[ERROR] invalid IBFT message retrieved", "error", err, "message", msg)

		return
	}

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

		msgs.mux.Unlock()
	}
}

// GetEvidences returns double signing evidences for the given height
func (t *DoubleSigningTrackerImpl) GetEvidences(height uint64) []*DoubleSignEvidence {
	evidences := []*DoubleSignEvidence{}

	for _, msgType := range t.msgsTypes {
		msgs := t.resolveMessagesStorage(msgType)
		if msgs == nil {
			continue
		}

		msgs.mux.Lock()

		roundMsgs, ok := msgs.content[height]
		if !ok {
			continue
		}

		for round, senderMsgs := range roundMsgs {
			for address, msgs := range senderMsgs {
				if len(msgs) <= 1 {
					continue
				}

				var evidence *DoubleSignEvidence

				firstMsg := msgs[0]
				for _, msg := range msgs[1:] {
					if !bytes.Equal(firstMsg.Signature, msg.Signature) {
						if evidence == nil {
							evidence = newDoubleSignEvidence(address, round, []*ibftProto.Message{})
							evidences = append(evidences, evidence)
						}

						evidence.messages = append(evidence.messages, msg)
					}
				}

				evidence.messages = append([]*ibftProto.Message{firstMsg}, evidence.messages...)
			}
		}

		msgs.mux.Unlock()
	}

	return evidences
}

// PostBlock is used to populate all known validators
func (t *DoubleSigningTrackerImpl) PostBlock(_ *common.PostBlockRequest) error {
	validators, err := t.validatorsProvider.GetValidators()
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
