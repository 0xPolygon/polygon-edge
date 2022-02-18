package bridge

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sync"

	"github.com/0xPolygon/polygon-edge/bridge/sam"
	"github.com/0xPolygon/polygon-edge/bridge/tracker"
	"github.com/0xPolygon/polygon-edge/bridge/transport"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/go-web3"
)

type Bridge interface {
	Start() error
	Close() error
	SetValidators([]types.Address, uint64)
	GetReadyMessages() []sam.MessageAndSignatures
	Consume(uint64)
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
		DBPath:        fmt.Sprintf("%s/last-processed-block", dataDirURL),
		ContractABIs: map[string][]string{
			types.AddressToString(config.RootChainContract): {
				`
				event RegistrationUpdated(
					address indexed user,
					address indexed sender,
					address indexed receiver
				)
				`,
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

func (b *bridge) GetReadyMessages() []sam.MessageAndSignatures {
	return b.sampool.GetReadyMessages()
}

func (b *bridge) Consume(id uint64) {
	b.sampool.Consume(id)
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
			if err := b.processLog(data); err != nil {
				b.logger.Error("failed to process event", "err", err)
			}
		}
	}
}

func (b *bridge) processLog(data []byte) error {
	// workaround
	var log web3.Log
	if err := json.Unmarshal(data, &log); err != nil {
		return err
	}

	// TODO: fix
	id := big.NewInt(0)

	if err := b.addLocalMessage(id.Uint64(), data); err != nil {
		return err
	}

	return nil
}

func (b *bridge) addLocalMessage(id uint64, body []byte) error {
	// XXX: sign hash instead of body
	signature, err := b.signer.Sign(body)
	if err != nil {
		return err
	}

	signedMessage := &sam.SignedMessage{
		Message: sam.Message{
			ID:   id,
			Body: body,
		},
		Address:   b.signer.Address(),
		Signature: signature,
	}

	b.sampool.MarkAsKnown(id)
	b.sampool.Add(signedMessage)

	if err := b.transport.Publish(&signedMessage.Message, signature); err != nil {
		return err
	}

	return nil
}

func (b *bridge) addRemoteMessage(message *sam.Message, signature []byte) {
	sender, err := b.signer.RecoverAddress(message.Body, signature)
	if err != nil {
		b.logger.Error("failed to get address from signature", "err", err)

		return
	}

	if !b.isValidator(sender) {
		b.logger.Warn("ignored gossip message from non-validator", "ID", message.ID, "from", types.AddressToString(sender))

		return
	}

	b.sampool.Add(&sam.SignedMessage{
		Message:   *message,
		Address:   sender,
		Signature: signature,
	})
}
