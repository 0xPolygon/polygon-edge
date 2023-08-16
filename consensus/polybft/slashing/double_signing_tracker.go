package slashing

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"sync"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/common"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
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

var _ sort.Interface = (*SortedAddresses)(nil)

// SortedAddresses is a slice which holds addresses in an ascending order
type SortedAddresses []types.Address

func (s SortedAddresses) Len() int           { return len(s) }
func (s SortedAddresses) Less(i, j int) bool { return bytes.Compare(s[i].Bytes(), s[j].Bytes()) < 0 }
func (s SortedAddresses) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (s *SortedAddresses) Add(addr types.Address) {
	addrRaw := addr.Bytes()
	insertIdx := sort.Search(len(*s), func(i int) bool {
		return bytes.Compare((*s)[i].Bytes(), addrRaw) >= 0
	})

	if insertIdx < len(*s) && (*s)[insertIdx] == addr {
		return
	}

	*s = append(*s, types.ZeroAddress)
	copy((*s)[insertIdx+1:], (*s)[insertIdx:])
	(*s)[insertIdx] = addr
}

var _ sort.Interface = (*SortedRounds)(nil)

// SortedRounds is a slice which holds rounds in an ascending order
type SortedRounds []uint64

func (s SortedRounds) Len() int           { return len(s) }
func (s SortedRounds) Less(i, j int) bool { return s[i] < s[j] }
func (s SortedRounds) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (s *SortedRounds) Add(round uint64) {
	insertIdx := sort.Search(len(*s), func(i int) bool {
		return (*s)[i] >= round
	})

	if insertIdx < len(*s) && (*s)[insertIdx] == round {
		return
	}

	*s = append(*s, 0)
	copy((*s)[insertIdx+1:], (*s)[insertIdx:])
	(*s)[insertIdx] = round
}

type SenderMessagesMap map[types.Address][]*ibftProto.Message
type MessagesMap map[uint64]map[uint64]SenderMessagesMap

type DoubleSigners []types.Address

func (s *DoubleSigners) EncodeAbi() ([]byte, error) {
	validators := make([]ethgo.Address, len(*s))

	for i, address := range *s {
		validators[i] = ethgo.Address(address)
	}

	slashFn := &contractsapi.SlashValidatorSetFn{
		Validators: validators,
	}

	return slashFn.EncodeAbi()
}

// contains checks whether provided sender is already present among the double signers
func (s *DoubleSigners) contains(sender types.Address) bool {
	for _, addr := range *s {
		if addr == sender {
			return true
		}
	}

	return false
}

type Messages struct {
	content       MessagesMap
	sortedSenders map[uint64]SortedAddresses
	sortedRounds  map[uint64]SortedRounds
	mux           sync.RWMutex
}

func (m *Messages) String() string {
	var buf bytes.Buffer

	buf.WriteString("senders")
	buf.WriteString(fmt.Sprintf("\t%v\n", m.sortedSenders))
	buf.WriteString("rounds")
	buf.WriteString(fmt.Sprintf("\t%v\n", m.sortedRounds))
	buf.WriteString("messages")
	buf.WriteString(fmt.Sprintf("\t%v\n", m.content))

	return buf.String()
}

// newMessages is a constructor function for Messages struct
func newMessages() *Messages {
	return &Messages{
		content:       make(MessagesMap),
		sortedSenders: make(map[uint64]SortedAddresses),
		sortedRounds:  make(map[uint64]SortedRounds),
	}
}

func (m *Messages) Len() int {
	return len(m.content)
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

	msgs = append(msgs, msg)
	senderMsgs[sender] = msgs

	// add sender into sorted sender slice
	senders, ok := m.sortedSenders[view.Height]
	if !ok {
		senders = make(SortedAddresses, 0, 1)
	}

	senders.Add(sender)
	m.sortedSenders[view.Height] = senders

	// add round into sorted rounds slice
	rounds, ok := m.sortedRounds[view.Height]
	if !ok {
		rounds = make(SortedRounds, 0, 1)
	}

	rounds.Add(view.Round)
	m.sortedRounds[view.Height] = rounds
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
		preprepare:         newMessages(),
		prepare:            newMessages(),
		commit:             newMessages(),
		roundChange:        newMessages(),
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

// GetDoubleSigners returns double signers for the given height
func (t *DoubleSigningTrackerImpl) GetDoubleSigners(height uint64) DoubleSigners {
	doubleSigners := make(DoubleSigners, 0)

	for _, msgType := range t.msgsTypes {
		msgs := t.resolveMessagesStorage(msgType)
		if msgs == nil || msgs.Len() == 0 {
			continue
		}

		msgs.mux.Lock()

		roundMsgs, msgsMapExists := msgs.content[height]
		senders, sortedSendersExist := msgs.sortedSenders[height]
		rounds, sortedRoundsExist := msgs.sortedRounds[height]

		if !msgsMapExists || !sortedSendersExist || !sortedRoundsExist {
			msgs.mux.Unlock()

			continue
		}

		for _, round := range rounds {
			msgsPerSenders := roundMsgs[round]

			for _, sender := range senders {
				msgs, ok := msgsPerSenders[sender]
				if !ok || len(msgs) < 2 || doubleSigners.contains(sender) {
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
func (t *DoubleSigningTrackerImpl) PostBlock(req *common.PostBlockRequest) error {
	t.PruneMsgsUntil(req.FullBlock.Block.Number())

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
			return fmt.Errorf("sender %s is detected as a spammer (same message was already sent)", sender)
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
