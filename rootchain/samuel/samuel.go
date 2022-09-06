package samuel

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/blockchain/storage"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/rootchain/payload"
	"github.com/0xPolygon/polygon-edge/rootchain/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo/abi"
	googleProto "google.golang.org/protobuf/proto"
)

// eventTracker defines the event tracker interface for SAMUEL
type eventTracker interface {
	// Start starts the event tracker from the specified block number
	Start(uint64)

	// Stop stops the event tracker
	Stop()

	// Subscribe creates a rootchain event subscription
	Subscribe() <-chan rootchain.Event
}

// samp defines the SAMP interface for SAMUEL
type samp interface {
	// AddMessage pushes a Signed Arbitrary Message into the SAMP
	AddMessage(rootchain.SAM) error

	// Prune prunes out all SAMs based on the specified event index
	Prune(uint64)

	// Peek returns a ready set of SAM messages, without removal
	Peek() rootchain.VerifiedSAM

	// Pop returns a ready set of SAM messages, with removal
	Pop() rootchain.VerifiedSAM
}

// signer defines the signer interface used for
// generating signatures
type signer interface {
	// Sign signs the specified data
	Sign([]byte) ([]byte, error)
}

// transport defines the transport interface used for
// publishing and subscribing to gossip events
type transport interface {
	// Publish gossips the specified SAM message
	Publish(*proto.SAM) error

	// Subscribe subscribes for incoming SAM messages
	Subscribe(func(*proto.SAM)) error
}

// eventData holds information on event data mapping
type eventData struct {
	payloadType rootchain.PayloadType
	eventABI    *abi.Event
	methodABI   *abi.ABI
}

// SAMUEL is the module that coordinates activities with the SAMP and Event Tracker
type SAMUEL struct {
	// eventLookup maps the local Smart Contract address to the event data
	eventLookup map[types.Address]eventData
	logger      hclog.Logger

	eventTracker eventTracker
	samp         samp
	storage      storage.Storage
	signer       signer
	transport    transport
}

// NewSamuel creates a new SAMUEL instance
func NewSamuel(
	config *rootchain.Config,
	logger hclog.Logger,
	eventTracker eventTracker,
	samp samp,
	signer signer,
	storage storage.Storage,
	transport transport,
) *SAMUEL {
	return &SAMUEL{
		logger:       logger.Named("SAMUEL"),
		eventLookup:  initEventLookupMap(config),
		eventTracker: eventTracker,
		samp:         samp,
		signer:       signer,
		storage:      storage,
		transport:    transport,
	}
}

// initEventLookupMap generates the SAMUEL event data lookup map from the
// passed in rootchain configuration
func initEventLookupMap(
	config *rootchain.Config,
) map[types.Address]eventData {
	lookupMap := make(map[types.Address]eventData)

	for rootchainAddr := range config.RootchainAddresses {
		// Grab the config events for the specific rootchain WS address
		configEvents := config.RootchainAddresses[rootchainAddr]

		// Initialize the lookup map with these events
		for _, chainEvent := range configEvents {
			lookupMap[types.StringToAddress(chainEvent.LocalAddress)] = eventData{
				payloadType: chainEvent.PayloadType,
				eventABI:    abi.MustNewEvent(chainEvent.EventABI),
				methodABI:   abi.MustNewABI(chainEvent.MethodABI),
			}
		}
	}

	return lookupMap
}

func (s *SAMUEL) Start() error {
	// Start the event loop for the tracker
	s.startEventLoop()

	return s.registerGossipHandler()
}

func (s *SAMUEL) registerGossipHandler() error {
	return s.transport.Subscribe(func(sam *proto.SAM) {
		// Extract the event data
		eventPayload, err := s.getEventPayload(sam.Event.Payload, sam.Event.PayloadType)
		if err != nil {
			s.logger.Warn(
				fmt.Sprintf("unable to get event payload with hash %s, %v", sam.Hash, err),
			)

			return
		}

		// Convert the proto event to a local SAM
		localSAM := rootchain.SAM{
			Hash:      types.BytesToHash(sam.Hash),
			Signature: sam.Signature,
			Event: rootchain.Event{
				Index:       sam.Event.Index,
				BlockNumber: sam.Event.BlockNumber,
				Payload:     eventPayload,
			},
		}

		if err := s.samp.AddMessage(localSAM); err != nil {
			s.logger.Warn(
				fmt.Sprintf("unable to add event with hash %s to SAMP, %v", sam.Hash, err),
			)
		}
	})
}

func (s *SAMUEL) getEventPayload(
	eventPayload []byte,
	payloadType uint64,
) (rootchain.Payload, error) {
	switch rootchain.PayloadType(payloadType) {
	case rootchain.ValidatorSetPayloadType:
		vsProto := &proto.ValidatorSetPayload{}
		if err := googleProto.Unmarshal(eventPayload, vsProto); err != nil {
			return nil, fmt.Errorf("unable to unmarshal proto payload, %v", err)
		}
		setInfo := make([]payload.ValidatorSetInfo, len(vsProto.ValidatorsInfo))

		for index, info := range vsProto.ValidatorsInfo {
			setInfo[index] = payload.ValidatorSetInfo{
				Address:      info.Address,
				BLSPublicKey: info.BlsPubKey,
			}
		}

		return payload.NewValidatorSetPayload(setInfo), nil
	default:
		return nil, errors.New("unknown payload type")
	}
}

func (s *SAMUEL) startEventLoop() {
	subscription := s.eventTracker.Subscribe()

	go func() {
		for ev := range subscription {
			// Get the raw event data as bytes
			data, err := ev.Marshal()
			if err != nil {
				s.logger.Warn(fmt.Sprintf("unable to marshal Event Tracker event, %v", err))

				continue
			}

			// Get the hash and the signature of the event
			hash := crypto.Keccak256(data)
			signature, err := s.signer.Sign(hash)

			if err != nil {
				s.logger.Warn(fmt.Sprintf("unable to sign Event Tracker event, %v", err))

				continue
			}

			// Push the SAM to the local SAMP
			sam := rootchain.SAM{
				Hash:      types.BytesToHash(hash),
				Signature: signature,
				Event:     ev,
			}

			if err := s.samp.AddMessage(sam); err != nil {
				s.logger.Warn(fmt.Sprintf("unable to add event with hash %s to SAMP, %v", sam.Hash, err))

				continue
			}

			// Publish the signature for other nodes
			if err := s.transport.Publish(sam.ToProto()); err != nil {
				s.logger.Warn(
					fmt.Sprintf("unable to publish SAM message with hash %s to SAMP, %v", sam.Hash, err),
				)

				continue
			}
		}
	}()
}

func (s *SAMUEL) Stop() {

}

func (s *SAMUEL) SaveProgress(contractAddr types.Address, input []byte) {

}

func (s *SAMUEL) GetReadyTransactions() []types.Transaction {
	return nil
}
