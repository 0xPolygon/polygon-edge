package slashing

import (
	"bytes"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
)

type SenderMessagesMap map[types.Address][]*ibftProto.Message

type DoubleSignEvidence struct {
	signer   types.Address
	round    uint64
	messages []*ibftProto.Message
}

func newDoubleSignEvidence(signer types.Address, round uint64,
	messages []*ibftProto.Message) *DoubleSignEvidence {
	return &DoubleSignEvidence{signer: signer, round: round, messages: messages}
}

type MessagesMap map[uint64]map[uint64]SenderMessagesMap

// getSenderMsgs returns commit messages for given height, round and sender
func (m MessagesMap) getSenderMsgs(view *ibftProto.View, sender types.Address) []*ibftProto.Message {
	if roundMsgs, ok := m[view.Height]; ok {
		if senderMsgsMap, ok := roundMsgs[view.Round]; ok {
			return senderMsgsMap[sender]
		}
	}

	return nil
}

// registerSenderMsgs registers messages for the given height, round and sender
func (m MessagesMap) registerSenderMsgs(view *ibftProto.View, sender types.Address, msgs []*ibftProto.Message) {
	var senderMsgs SenderMessagesMap

	if roundMsgs, ok := m[view.Height]; !ok {
		roundMsgs = make(map[uint64]SenderMessagesMap)
		m[view.Height] = roundMsgs
		senderMsgs = createSenderMsgsMap(view.Round, roundMsgs)
	} else if senderMsgs, ok = roundMsgs[view.Round]; !ok {
		senderMsgs = createSenderMsgsMap(view.Round, roundMsgs)
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
	preprepare  MessagesMap
	prepare     MessagesMap
	commit      MessagesMap
	roundChange MessagesMap

	logger hclog.Logger
}

func NewDoubleSigningTracker(logger hclog.Logger) *DoubleSigningTrackerImpl {
	return &DoubleSigningTrackerImpl{
		logger:      logger,
		preprepare:  make(MessagesMap),
		prepare:     make(MessagesMap),
		commit:      make(MessagesMap),
		roundChange: make(MessagesMap),
	}
}

// Handle is implementation of IBFTMessageHandler interface, which handles IBFT consensus messages
func (t *DoubleSigningTrackerImpl) Handle(msg *ibftProto.Message) {
	signer, err := wallet.RecoverSignerFromIBFTMessage(msg)
	if err != nil {
		t.logger.Debug("[ERROR] failed to recover signer upon message receive",
			"message", msg)

		return
	}

	sender := types.BytesToAddress(msg.From)
	// ignore messages where signer and sender are not the same
	if signer != sender {
		t.logger.Debug("[ERROR] signer and sender of IBFT message are not the same, ignoring it...",
			"message", msg)

		return
	}

	msgMap := t.resolveMsgMap(msg.GetType())

	senderMsgs := msgMap.getSenderMsgs(msg.View, sender)
	if senderMsgs == nil {
		senderMsgs = []*ibftProto.Message{msg}
	} else {
		senderMsgs = append(senderMsgs, msg)
	}

	msgMap.registerSenderMsgs(msg.View, sender, senderMsgs)
}

// PruneMsgsUntil deletes all messages maps until the specified height
func (t *DoubleSigningTrackerImpl) PruneMsgsUntil(height uint64) {
	for _, msgType := range ibftProto.MessageType_value {
		msgsMap := t.resolveMsgMap(ibftProto.MessageType(msgType))
		if msgsMap == nil {
			continue
		}

		for msgHeight := range msgsMap {
			if msgHeight < height {
				delete(msgsMap, msgHeight)
			}
		}
	}
}

// GetEvidences returns double signing evidences for the given height
func (t *DoubleSigningTrackerImpl) GetEvidences(height uint64) []*DoubleSignEvidence {
	var (
		evidences = []*DoubleSignEvidence{}
		evidence  *DoubleSignEvidence
	)

	for _, msgType := range ibftProto.MessageType_value {
		msgsMap := t.resolveMsgMap(ibftProto.MessageType(msgType))
		if msgsMap == nil {
			continue
		}

		roundMsgs, ok := msgsMap[height]
		if !ok {
			continue
		}

		for round, senderMsgs := range roundMsgs {
			for address, msgs := range senderMsgs {
				if len(msgs) <= 1 {
					continue
				}

				for _, msg := range msgs[1:] {
					if bytes.Equal(msgs[0].Signature, msg.Signature) {
						if evidence == nil {
							evidence = newDoubleSignEvidence(address, round, []*ibftProto.Message{})
							evidences = append(evidences, evidence)
						}

						evidence.messages = append(evidence.messages, msg)
					}
				}
			}
		}
	}

	return evidences
}

// resolveMsgMap resolves proper messages map based on the provided message type
func (t *DoubleSigningTrackerImpl) resolveMsgMap(msgType ibftProto.MessageType) MessagesMap {
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
