package bridge

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/bridge/sam"
	"github.com/0xPolygon/polygon-edge/bridge/tracker"
	"github.com/0xPolygon/polygon-edge/bridge/transport"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
)

const (
	StateSyncedEventABI = `event StateSynced(uint256 indexed id, address indexed contractAddress, bytes data)`
)

var (
	StateSyncedEvent = abi.MustNewEvent(StateSyncedEventABI)
)

type Bridge interface {
	Start() error
	Close() error
	SetValidators([]types.Address, uint64)
	GetReadyMessages() ([]MessageWithSignatures, error)
	ValidateTx(*types.Transaction) error
	Consume(*types.Transaction)
}

type bridge struct {
	logger hclog.Logger

	signer sam.Signer

	// network
	tracker   *tracker.Tracker
	transport transport.MessageTransport

	isValidatorMapLock sync.RWMutex
	isValidatorMap     map[types.Address]bool
	validatorThreshold uint64

	// storage
	sampool sam.Pool

	closeCh chan struct{}
}

func NewBridge(
	logger hclog.Logger,
	network *network.Server,
	signer sam.Signer,
	dataDirURL string,
	config *Config,
) (Bridge, error) {
	bridgeLogger := logger.Named("bridge")

	trackerConfig := &tracker.Config{
		Confirmations: config.Confirmations,
		RootchainWS:   config.RootChainURL.String(),
		DBPath:        dataDirURL,
		ContractABIs: map[string][]string{
			config.RootChainContract.String(): {
				StateSyncedEventABI,
			},
		},
	}

	tracker, err := tracker.NewEventTracker(bridgeLogger, trackerConfig)
	if err != nil {
		return nil, err
	}

	return &bridge{
		logger:             bridgeLogger,
		signer:             signer,
		tracker:            tracker,
		transport:          transport.NewLibp2pGossipTransport(logger, network),
		isValidatorMapLock: sync.RWMutex{},
		isValidatorMap:     map[types.Address]bool{},
		sampool:            sam.NewPool(nil, 0),
		closeCh:            make(chan struct{}),
	}, nil
}

func (b *bridge) Start() error {
	if err := b.transport.Start(); err != nil {
		return err
	}

	if err := b.transport.Subscribe(b.addRemoteMessage); err != nil {
		return err
	}

	if err := b.tracker.Start(); err != nil {
		return err
	}

	eventCh := b.tracker.GetEventChannel()
	go b.processEvents(eventCh)

	return nil
}

func (b *bridge) Close() error {
	close(b.closeCh)

	if err := b.tracker.Stop(); err != nil {
		return err
	}

	return nil
}

func (b *bridge) SetValidators(validators []types.Address, threshold uint64) {
	b.resetIsValidatorMap(validators)
	b.validatorThreshold = threshold

	b.sampool.UpdateValidatorSet(validators, threshold)
}

func (b *bridge) GetReadyMessages() ([]MessageWithSignatures, error) {
	readyMessages := b.sampool.GetReadyMessages()

	data := make([]MessageWithSignatures, 0, len(readyMessages))

	for _, readyMsg := range readyMessages {
		msg, ok := readyMsg.Data.(*Message)
		if !ok {
			return nil, fmt.Errorf("get unknown type data %T when fetching ready messages", readyMsg.Data)
		}

		data = append(data, MessageWithSignatures{
			Message:    *msg,
			Signatures: readyMsg.Signatures,
		})
	}

	return data, nil
}

func (b *bridge) ValidateTx(tx *types.Transaction) error {
	hash := getTransactionHash(tx)

	if !b.sampool.Knows(hash) {
		return fmt.Errorf("unknown state transaction, hash=%s", hash.String())
	}

	num, required := b.sampool.GetSignatureCount(hash), b.validatorThreshold
	if num < required {
		return fmt.Errorf("Bridge doesn't have enough signatures, hash=%s, required=%d, actual=%d", hash, required, num)
	}

	return nil
}

func (b *bridge) Consume(tx *types.Transaction) {
	b.sampool.Consume(getTransactionHash(tx))
}

func (b *bridge) resetIsValidatorMap(validators []types.Address) {
	isValidatorMap := make(map[types.Address]bool)
	for _, address := range validators {
		isValidatorMap[address] = true
	}

	b.isValidatorMapLock.Lock()
	defer b.isValidatorMapLock.Unlock()

	b.isValidatorMap = isValidatorMap
}

func (b *bridge) isValidator(address types.Address) bool {
	b.isValidatorMapLock.RLock()
	defer b.isValidatorMapLock.RUnlock()

	return b.isValidatorMap[address]
}

func (b *bridge) processEvents(eventCh <-chan []byte) {
	for {
		select {
		case <-b.closeCh:
			return
		case data := <-eventCh:
			if err := b.processEthEvent(data); err != nil {
				b.logger.Error("failed to process event", "err", err)
			}
		}
	}
}

func (b *bridge) processEthEvent(data []byte) error {
	var log web3.Log
	if err := json.Unmarshal(data, &log); err != nil {
		return err
	}

	msg, err := eventToMessage(&log)
	if err != nil {
		return err
	}

	if msg == nil {
		return fmt.Errorf("unknown event: tx=%s, log index=%d", log.TransactionHash, log.LogIndex)
	}

	if err := b.addLocalMessage(msg); err != nil {
		return err
	}

	return nil
}

func (b *bridge) addLocalMessage(msg *Message) error {
	hash := getMessageHash(msg)

	signature, err := b.signer.Sign(hash[:])
	if err != nil {
		return err
	}

	b.sampool.AddMessage(&sam.Message{
		Hash: hash,
		Data: msg,
	})

	b.sampool.AddSignature(&sam.MessageSignature{
		Hash:      hash,
		Address:   b.signer.Address(),
		Signature: signature,
	})

	signedMessage := &transport.SignedMessage{
		Hash:      hash,
		Signature: signature,
	}

	if err := b.transport.Publish(signedMessage); err != nil {
		return err
	}

	return nil
}

func (b *bridge) addRemoteMessage(message *transport.SignedMessage) {
	sender, err := b.signer.RecoverAddress(message.Hash[:], message.Signature)
	if err != nil {
		b.logger.Error("failed to get address from signature", "err", err)

		return
	}

	if !b.isValidator(sender) {
		b.logger.Warn(
			"ignored gossip message from non-validator",
			"hash",
			message.Hash,
			"from",
			types.AddressToString(sender),
		)

		return
	}

	b.sampool.AddSignature(&sam.MessageSignature{
		Hash:      message.Hash,
		Address:   sender,
		Signature: message.Signature,
	})
}

func getMessageHash(msg *Message) types.Hash {
	// return msg.ComputeHash()
	return msg.Transaction.ComputeHash().Hash
}

func getTransactionHash(tx *types.Transaction) types.Hash {
	msgTx := &types.Transaction{
		Type:  types.TxTypeState,
		To:    tx.To,
		Input: tx.Input,
	}

	return msgTx.ComputeHash().Hash
}

func eventToMessage(log *web3.Log) (*Message, error) {
	switch {
	case StateSyncedEvent.Match(log):
		event, err := ParseStateSyncEvent(log)
		if err != nil {
			return nil, err
		}

		tx := NewStateSyncedTx(event)

		return &Message{
			ID:          event.ID,
			Transaction: tx,
		}, nil
	}

	return nil, nil
}
