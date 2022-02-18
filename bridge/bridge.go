package bridge

import (
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/bridge/sam"
	"github.com/0xPolygon/polygon-edge/bridge/tracker"
	"github.com/0xPolygon/polygon-edge/bridge/transport"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	StateSyncedABI = `event StateSynced(uint256 indexed id, address indexed contractAddress, bytes data)`
)

type Bridge interface {
	Start() error
	Close() error
	SetValidators([]types.Address, uint64)
	GetReadyMessages() []sam.ReadyMessage
	Consume(types.Hash)
}

type bridge struct {
	logger hclog.Logger

	signer sam.Signer

	// network
	tracker   *tracker.Tracker
	transport transport.MessageTransport

	isValidatorMapLock sync.RWMutex
	isValidatorMap     map[types.Address]bool

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
	fmt.Printf("NewBridge, address=%+v, config=%+v\n", signer.Address(), config)

	bridgeLogger := logger.Named("bridge")

	trackerConfig := &tracker.Config{
		Confirmations: config.Confirmations,
		RootchainWS:   config.RootChainURL.String(),
		DBPath:        dataDirURL,
		ContractABIs: map[string][]string{
			config.RootChainContract.String(): {
				StateSyncedABI,
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

	b.tracker.Start()

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
	b.sampool.UpdateValidatorSet(validators, threshold)
}

func (b *bridge) GetReadyMessages() []sam.ReadyMessage {
	return b.sampool.GetReadyMessages()
}

func (b *bridge) Consume(hash types.Hash) {
	b.sampool.Consume(hash)
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
			if err := b.addLocalMessage(data); err != nil {
				b.logger.Error("failed process event", "err", err)
			}
		}
	}
}

func (b *bridge) addLocalMessage(data []byte) error {
	hash := types.BytesToHash(crypto.Keccak256(data))
	signature, err := b.signer.Sign(hash[:])
	if err != nil {
		return err
	}

	fmt.Printf("addLocalMessage hash=%+v,signature=%+v, data=%+v\n", hash, signature, data)

	b.sampool.AddMessage(&sam.Message{
		Hash: hash,
		Body: data,
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

	fmt.Printf("addRemoteMessage hash=%+v, signature=%+v, sender=%+v\n", message.Hash, message.Signature, sender)

	if !b.isValidator(sender) {
		b.logger.Warn("ignored gossip message from non-validator", "hash", message.Hash, "from", types.AddressToString(sender))

		return
	}

	b.sampool.AddSignature(&sam.MessageSignature{
		Hash:      message.Hash,
		Address:   sender,
		Signature: message.Signature,
	})
}
