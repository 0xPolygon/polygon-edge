package slashing

import (
	"bytes"
	"errors"
	"sync"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	errViewUndefined           = errors.New("view is undefined")
	errInvalidMsgType          = errors.New("message type is invalid")
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

// getSenderMsgsLocked returns messages for given height, round and sender
func (m *Messages) getSenderMsgsLocked(view *ibftProto.View, sender types.Address) []*ibftProto.Message {
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

	msgs := m.getSenderMsgsLocked(msg.View, sender)
	if msgs == nil {
		msgs = []*ibftProto.Message{msg}
	} else {
		msgs = append(msgs, msg)
	}

	senderMsgs[sender] = msgs
}

// createSenderMsgsMap initializes senders message map for the given round
func createSenderMsgsMap(round uint64, roundMsgs map[uint64]SenderMessagesMap) SenderMessagesMap {
	sendersMsgs := make(SenderMessagesMap)
	roundMsgs[round] = sendersMsgs

	return sendersMsgs
}

type DoubleSigningTracker interface {
	Handle(msg *ibftProto.Message)
	GetEvidences(height uint64) []*DoubleSignEvidence
	PruneMsgsUntil(height uint64)
}

type DoubleSigningTrackerImpl struct {
	preprepare  *Messages
	prepare     *Messages
	commit      *Messages
	roundChange *Messages

	logger hclog.Logger
}

func NewDoubleSigningTracker(logger hclog.Logger) *DoubleSigningTrackerImpl {
	return &DoubleSigningTrackerImpl{
		logger:      logger,
		preprepare:  &Messages{content: make(MessagesMap)},
		prepare:     &Messages{content: make(MessagesMap)},
		commit:      &Messages{content: make(MessagesMap)},
		roundChange: &Messages{content: make(MessagesMap)},
	}
}

// Handle is implementation of IBFTMessageHandler interface, which handles IBFT consensus messages
func (t *DoubleSigningTrackerImpl) Handle(msg *ibftProto.Message) {
	if err := t.validateMsg(msg); err != nil {
		t.logger.Debug("[ERROR] invalid IBFT message retrieved", "error", err, "message", msg)

		return
	}

	sender := types.BytesToAddress(msg.From)

	msgMap := t.resolveMessagesStorage(msg.GetType())
	msgMap.addMessage(msg.View, sender, msg)
}

// PruneMsgsUntil deletes all messages maps until the specified height
func (t *DoubleSigningTrackerImpl) PruneMsgsUntil(height uint64) {
	for _, msgType := range ibftProto.MessageType_value {
		msgs := t.resolveMessagesStorage(ibftProto.MessageType(msgType))
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

	for _, msgType := range ibftProto.MessageType_value {
		msgs := t.resolveMessagesStorage(ibftProto.MessageType(msgType))
		if msgs == nil {
			continue
		}

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
	}

	return evidences
}

// validateMsg validates provided IBFT message
func (t *DoubleSigningTrackerImpl) validateMsg(msg *ibftProto.Message) error {
	if msg.View == nil {
		return errViewUndefined
	}

	if _, ok := ibftProto.MessageType_name[int32(msg.Type)]; !ok {
		return errInvalidMsgType
	}

	signer, err := wallet.RecoverSignerFromIBFTMessage(msg)
	if err != nil {
		return err
	}

	// ignore messages where signer and sender are not the same
	if signer != types.BytesToAddress(msg.From) {
		return errSignerAndSenderMismatch
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
