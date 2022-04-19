package statesync

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/bridge/sam"
	"github.com/0xPolygon/polygon-edge/bridge/statesync/transport"
	"github.com/0xPolygon/polygon-edge/bridge/tracker"
	"github.com/0xPolygon/polygon-edge/bridge/utils"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/go-web3"
)

type StateSync interface {
	Start() error
	Close() error

	GetReadyMessages() ([]MessageWithSignatures, error)
	GetTransactionHash(*types.Transaction) (types.Hash, error)
	Consume(*types.Transaction) error
}

type stateSync struct {
	logger hclog.Logger

	signer       sam.Signer
	validatorSet utils.ValidatorSet
	sampool      sam.Pool

	tracker   *tracker.Tracker
	transport transport.MessageTransport

	closeCh chan struct{}
}

func NewStateSync(
	logger hclog.Logger,
	network *network.Server,
	signer sam.Signer,
	validatorSet utils.ValidatorSet,
	dataDirURL string,
	rootchainURL string,
	rootchainContract types.Address,
	confirmations uint64,
) (StateSync, error) {
	statesyncLogger := logger.Named("state-sync")

	trackerConfig := &tracker.Config{
		Confirmations: confirmations,
		RootchainWS:   rootchainURL,
		DBPath:        dataDirURL,
		ContractABIs: map[string][]string{
			rootchainContract.String(): {
				StateSyncedEventABI,
			},
		},
	}

	tracker, err := tracker.NewEventTracker(statesyncLogger, trackerConfig)
	if err != nil {
		return nil, err
	}

	return &stateSync{
		logger:       statesyncLogger,
		signer:       signer,
		validatorSet: validatorSet,
		tracker:      tracker,
		transport:    transport.NewLibp2pGossipTransport(logger, network),
		sampool:      sam.NewPool(validatorSet),
		closeCh:      make(chan struct{}),
	}, nil
}

func (b *stateSync) Start() error {
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

func (b *stateSync) Close() error {
	close(b.closeCh)

	if err := b.tracker.Stop(); err != nil {
		return err
	}

	return nil
}

func (b *stateSync) GetReadyMessages() ([]MessageWithSignatures, error) {
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

func (b *stateSync) GetTransactionHash(tx *types.Transaction) (types.Hash, error) {
	return getTransactionHash(tx)
}

func (b *stateSync) Consume(tx *types.Transaction) error {
	txHash, err := getTransactionHash(tx)
	if err != nil {
		return err
	}

	b.sampool.ConsumeMessage(txHash)

	return nil
}

func (b *stateSync) processEvents(eventCh <-chan []byte) {
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

func (b *stateSync) processEthEvent(data []byte) error {
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

func (b *stateSync) addLocalMessage(msg *Message) error {
	hash := msg.Hash()

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

	b.logger.Info("added local signature to SAM Pool", "hash", hash)

	signedMessage := &transport.SignedMessage{
		Hash:      hash,
		Signature: signature,
	}

	if err := b.transport.Publish(signedMessage); err != nil {
		return err
	}

	return nil
}

func (b *stateSync) addRemoteMessage(message *transport.SignedMessage) {
	sender, err := b.signer.RecoverAddress(message.Hash[:], message.Signature)
	if err != nil {
		b.logger.Error("failed to get address from signature", "err", err)

		return
	}

	fmt.Printf("Received signature from other validator: sender=%s\n", sender.String())

	if !b.validatorSet.IsValidator(sender) {
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

	b.logger.Info("added remote signature to SAM Pool", "hash", message.Hash, "from", sender)
}
