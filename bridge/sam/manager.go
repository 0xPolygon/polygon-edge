package sam

import (
	"sync"

	"github.com/0xPolygon/polygon-edge/bridge/sam/proto"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

// Define the SAM libp2p protocol
var samProto = "/sam/0.1"

type manager struct {
	logger hclog.Logger // Output logger

	// signing
	signer    Signer
	recoverer SignatureRecoverer

	// network
	network *network.Server
	topic   *network.Topic

	// sam store
	pool Pool

	// TODO
	eventTracker    interface{}
	isValidatorLock sync.RWMutex
	isValidator     map[types.Address]bool
}

func NewManager(
	logger hclog.Logger,
	signer Signer,
	recoverer SignatureRecoverer,
	network *network.Server,
	initialValidators []types.Address,
	initialThreshold uint64,

) Manager {
	isValidator := make(map[types.Address]bool, len(initialValidators))
	for _, v := range initialValidators {
		isValidator[v] = true
	}

	return &manager{
		logger:          logger.Named("sam"),
		signer:          signer,
		recoverer:       recoverer,
		network:         network,
		pool:            NewPool(initialValidators, initialThreshold),
		eventTracker:    nil, // TODO
		isValidatorLock: sync.RWMutex{},
		isValidator:     isValidator,
	}
}

func (m *manager) Start() error {
	if err := m.setupGossip(); err != nil {
		return err
	}

	return nil
}

func (m *manager) Close() error {
	m.topic = nil

	return nil
}

func (m *manager) AddMessage(message *Message) error {
	// XXX: sign hash instead of body
	signature, err := m.signer.Sign(message.Body)
	if err != nil {
		return err
	}

	m.pool.MarkAsKnown(message.ID)
	m.pool.Add(&SignedMessage{
		Message:   *message,
		Address:   m.signer.Address(),
		Signature: signature,
	})

	if err := m.gossipSignedMessage(message, signature); err != nil {
		m.logger.Warn("failed to gossip message", "ID", message.ID, "err", err)
	}

	return nil
}

func (m *manager) GetReadyMessages() []MessageAndSignatures {
	return m.pool.GetReadyMessages()
}

func (m *manager) UpdateValidatorSet(validators []types.Address, threshold uint64) {
	newIsValidator := make(map[types.Address]bool)
	for _, v := range validators {
		newIsValidator[v] = true
	}

	m.isValidatorLock.Lock()
	m.isValidator = newIsValidator
	m.isValidatorLock.Unlock()

	m.pool.UpdateValidatorSet(validators, threshold)
}

func (m *manager) setupGossip() error {
	topic, err := m.network.NewTopic(samProto, &proto.SignedMessage{})
	if err != nil {
		return err
	}

	if err := topic.Subscribe(m.handleGossippedMessage); err != nil {
		return err
	}

	m.topic = topic

	return nil
}

func (m *manager) gossipSignedMessage(message *Message, signature []byte) error {
	return m.topic.Publish(&proto.SignedMessage{
		Id:        message.ID,
		Body:      message.Body,
		Signature: signature,
	})
}

func (m *manager) handleGossippedMessage(obj interface{}) {
	msg, ok := obj.(*proto.SignedMessage)
	if !ok {
		m.logger.Error("invalid type assertion for message request")

		return
	}

	addr, err := m.recoverer.Recover(msg.Signature)
	if err != nil {
		m.logger.Error("failed to get address from signature", "err", err)

		return
	}

	m.isValidatorLock.RLock()
	isValidator := m.isValidator[addr]

	m.isValidatorLock.RUnlock()

	if !isValidator {
		m.logger.Warn("ignored gossip message from non-validator", "ID", msg.Id, "from", types.AddressToString(addr))

		return
	}

	m.pool.Add(&SignedMessage{
		Message: Message{
			ID:   msg.Id,
			Body: msg.Body,
		},
		Address:   addr,
		Signature: msg.Signature,
	})
}
