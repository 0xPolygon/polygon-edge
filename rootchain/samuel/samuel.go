package samuel

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/rootchain/payload"
	"github.com/0xPolygon/polygon-edge/rootchain/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo/abi"
	googleProto "google.golang.org/protobuf/proto"
)

var (
	errUnknownPayloadType = errors.New("unknown payload type")
)

// eventTracker defines the event tracker interface for SAMUEL
type eventTracker interface {
	// Start starts the event tracker from the specified block number
	Start(uint64) error

	// Stop stops the event tracker
	Stop() error

	// Subscribe creates a rootchain event subscription
	Subscribe() <-chan rootchain.Event
}

// samp defines the Signed Arbitrary Message Pool interface for SAMUEL
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
	// Sign signs the specified data,
	// and returns the signature and the block number at which
	// the signature was generated
	Sign([]byte) ([]byte, uint64, error)

	// VerifySignature verifies the signature for the passed in
	// raw data, and at the specified block number
	VerifySignature([]byte, []byte, uint64) error

	// Quorum returns the number of quorum validators
	// for the given block number
	Quorum(uint64) uint64
}

// transport defines the transport interface used for
// publishing and subscribing to gossip events
type transport interface {
	// Publish gossips the specified SAM message
	Publish(*proto.SAM) error

	// Subscribe subscribes for incoming SAM messages
	Subscribe(func(*proto.SAM)) error
}

// storage defines the required storage interface for SAMUEL
// and its modules
type storage interface {
	// ReadLastProcessedEvent reads the last processed event data
	ReadLastProcessedEvent(string) (string, bool)

	// WriteLastProcessedEvent writes the last processed event data
	WriteLastProcessedEvent(data string, contractAddr string) error
}

// eventData holds information on event data mapping
type eventData struct {
	payloadType  rootchain.PayloadType
	eventABI     *abi.Event
	methodABI    *abi.Method
	localAddress types.Address
}

// SAMUEL is the module that coordinates activities with the SAMP and Event Tracker
type SAMUEL struct {
	eventData eventData
	logger    hclog.Logger

	eventTracker eventTracker
	samp         samp
	storage      storage
	signer       signer
	transport    transport
}

// NewSamuel creates a new SAMUEL instance
func NewSamuel(
	configEvent *rootchain.ConfigEvent,
	logger hclog.Logger,
	eventTracker eventTracker,
	samp samp,
	signer signer,
	storage storage,
	transport transport,
) *SAMUEL {
	return &SAMUEL{
		logger:       logger.Named("SAMUEL"),
		eventData:    initEventData(configEvent),
		eventTracker: eventTracker,
		samp:         samp,
		signer:       signer,
		storage:      storage,
		transport:    transport,
	}
}

// initEventLookupMap generates the SAMUEL event data lookup map from the
// passed in rootchain configuration
func initEventData(
	configEvent *rootchain.ConfigEvent,
) eventData {
	return eventData{
		payloadType:  configEvent.PayloadType,
		eventABI:     abi.MustNewEvent(configEvent.EventABI),
		methodABI:    abi.MustNewABI(configEvent.MethodABI).GetMethod(configEvent.MethodName),
		localAddress: types.StringToAddress(configEvent.LocalAddress),
	}
}

// Start starts the SAMUEL module
func (s *SAMUEL) Start() error {
	// Start the event loop for the tracker
	s.startEventLoop()

	// Register the gossip message handler
	if err := s.registerGossipHandler(); err != nil {
		return fmt.Errorf("unable to register gossip handler, %w", err)
	}

	// Fetch the latest event data
	startBlock, err := s.getStartBlockNumber()
	if err != nil {
		return fmt.Errorf("unable to get start block number, %w", err)
	}

	// Start the Event Tracker
	if err := s.eventTracker.Start(startBlock); err != nil {
		return fmt.Errorf("unable to start event tracker, %w", err)
	}

	return nil
}

// getStartBlockNumber determines the starting block for the Event Tracker
func (s *SAMUEL) getStartBlockNumber() (uint64, error) {
	startBlock := rootchain.LatestRootchainBlockNumber

	data, exists := s.storage.ReadLastProcessedEvent(s.eventData.localAddress.String())
	if exists && data != "" {
		// index:blockNumber
		values := strings.Split(data, ":")
		if len(values) < 2 {
			return 0, fmt.Errorf("invalid last processed event in DB: %v", values)
		}

		blockNumber, err := strconv.ParseUint(values[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("unable to parse last processed block number in DB: %w", err)
		}

		startBlock = blockNumber
	}

	return startBlock, nil
}

// registerGossipHandler registers a listener for incoming SAM messages
// from other peers
func (s *SAMUEL) registerGossipHandler() error {
	return s.transport.Subscribe(func(sam *proto.SAM) {
		// Extract the event data
		eventPayload, err := getEventPayload(sam.Event.Payload, sam.Event.PayloadType)
		if err != nil {
			s.logger.Error(
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

		// Verify that the hash is correct
		marshalledEvent, err := localSAM.Event.Marshal()
		if err != nil {
			s.logger.Error(
				fmt.Sprintf("unable to marshal event, %v", err),
			)

			return
		}

		hash := crypto.Keccak256(marshalledEvent)
		if !bytes.Equal(sam.Hash, hash) {
			s.logger.Error("invalid hash for incoming event")

			return
		}

		// Verify that the signature is correct
		if err := s.signer.VerifySignature(
			hash,
			sam.Signature,
			sam.ChildchainBlockNumber,
		); err != nil {
			s.logger.Error(
				fmt.Sprintf("invalid signature for event with hash %s, %v", sam.Hash, err),
			)

			return
		}

		if err := s.samp.AddMessage(localSAM); err != nil {
			s.logger.Error(
				fmt.Sprintf("unable to add event with hash %s to SAMP, %v", sam.Hash, err),
			)
		}
	})
}

// getEventPayload retrieves a concrete payload implementation
// based on the passed in byte array and payload type
func getEventPayload(
	eventPayload []byte,
	payloadType uint64,
) (rootchain.Payload, error) {
	switch rootchain.PayloadType(payloadType) {
	case rootchain.ValidatorSetPayloadType:
		// Unmarshal the data
		vsProto := &proto.ValidatorSetPayload{}
		if err := googleProto.Unmarshal(eventPayload, vsProto); err != nil {
			return nil, fmt.Errorf("unable to unmarshal proto payload, %w", err)
		}

		setInfo := make([]payload.ValidatorSetInfo, len(vsProto.ValidatorsInfo))

		// Extract the specific info
		for index, info := range vsProto.ValidatorsInfo {
			setInfo[index] = payload.ValidatorSetInfo{
				Address:      info.Address,
				BLSPublicKey: info.BlsPubKey,
			}
		}

		// Return the specific Payload implementation
		return payload.NewValidatorSetPayload(setInfo), nil
	default:
		return nil, errUnknownPayloadType
	}
}

// startEventLoop starts the SAMUEL event monitoring loop, which retrieves
// events from the Event Tracker, bundles them, and sends them off to other nodes
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
			signature, blockNum, err := s.signer.Sign(hash)

			if err != nil {
				s.logger.Warn(fmt.Sprintf("unable to sign Event Tracker event, %v", err))

				continue
			}

			// Push the SAM to the local SAMP
			sam := rootchain.SAM{
				Hash:          types.BytesToHash(hash),
				Signature:     signature,
				ChildBlockNum: blockNum,
				Event:         ev,
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

// Stop stops the SAMUEL module and any underlying modules
func (s *SAMUEL) Stop() error {
	// Stop the Event Tracker
	if err := s.eventTracker.Stop(); err != nil {
		return fmt.Errorf(
			"unable to gracefully stop event tracker, %w",
			err,
		)
	}

	return nil
}

// SaveProgress notifies the SAMUEL module of which events
// are committed to the blockchain
func (s *SAMUEL) SaveProgress(
	contractAddr types.Address, // local Smart Contract address
	input []byte, // method with argument data
) {
	if contractAddr != types.StringToAddress(s.eventData.localAddress.String()) {
		s.logger.Warn(
			fmt.Sprintf("Attempted to save progress for unknown contract %s", contractAddr),
		)

		return
	}

	// Decode the inputs
	methodID := s.eventData.methodABI.ID()
	params, err := s.eventData.methodABI.Inputs.Decode(
		input[len(methodID):],
	)

	if err != nil {
		s.logger.Error(
			fmt.Sprintf("Unable to decode event params for contract %s, %v", contractAddr, err),
		)

		return
	}

	// Make sure it's of a correct type
	castParams, castErr := params.(map[string]interface{})
	if err != nil {
		s.logger.Error(
			fmt.Sprintf("Unable to properly cast input params, %v", castErr),
		)

		return
	}

	switch s.eventData.payloadType {
	case rootchain.ValidatorSetPayloadType:
		// The method needs to contain
		// (validatorSet[], index, blockNumber)
		index, _ := castParams["index"].(uint64)
		blockNumber, _ := castParams["blockNumber"].(uint64)

		// Save to the local database
		if err := s.storage.WriteLastProcessedEvent(
			fmt.Sprintf("%d:%d", index, blockNumber),
			contractAddr.String(),
		); err != nil {
			s.logger.Error(
				fmt.Sprintf(
					"Unable to save last processed event for contract %s, %v",
					contractAddr,
					err,
				),
			)

			return
		}

		// Realign the local SAMP
		s.samp.Prune(index)
	default:
		s.logger.Error("Unknown payload type")

		return
	}
}

// GetReadyTransaction retrieves the ready SAMP transaction which has
// enough valid signatures
func (s *SAMUEL) GetReadyTransaction() *types.Transaction {
	// Get the latest verified SAM
	verifiedSAM := s.samp.Peek()
	if verifiedSAM == nil {
		return nil
	}

	// Find the verified SAM that has the least quorum signatures
	verifiedSAM = s.getVerifiedSAMBucket(verifiedSAM)
	if verifiedSAM == nil {
		return nil
	}

	// Extract the required data
	SAM := []rootchain.SAM(verifiedSAM)[0]

	blockNumber := SAM.BlockNumber
	childBlockNumber := SAM.ChildBlockNum
	index := SAM.Index
	signatures := verifiedSAM.Signatures()

	// Extract the payload info
	payloadType, payloadData := SAM.Payload.Get()
	rawPayload, err := getEventPayload(payloadData, uint64(payloadType))

	if err != nil {
		s.logger.Error(
			fmt.Sprintf(
				"Unable to extract event payload for SAM %s, %v",
				SAM.Hash.String(),
				err,
			),
		)
	}

	switch payloadType {
	case rootchain.ValidatorSetPayloadType:
		// Get the validator set info
		vs, _ := rawPayload.(*payload.ValidatorSetPayload)
		setInfo := vs.GetSetInfo()
		validatorSetMap := make([]map[string][]byte, len(setInfo))

		for index, info := range setInfo {
			validatorSetMap[index] = map[string][]byte{
				"Address":      info.Address,
				"BLSPublicKey": info.BLSPublicKey,
			}
		}

		// The method should have the signature
		// methodName(validatorSet tuple[], index uint64, blockNumber uint64, signatures [][]byte)
		encodedArgs, err := s.eventData.methodABI.Inputs.Encode(
			map[string]interface{}{
				"validatorSet":         validatorSetMap,
				"index":                index,
				"blockNumber":          blockNumber,
				"signatures":           signatures,
				"signatureBlockNumber": childBlockNumber,
			},
		)

		if err != nil {
			s.logger.Error(
				fmt.Sprintf(
					"Unable to encode method arguments for SAM %s, %v",
					SAM.Hash.String(),
					err,
				),
			)

			return nil
		}

		return &types.Transaction{
			Nonce:    0,
			From:     types.ZeroAddress,
			To:       &s.eventData.localAddress,
			GasPrice: big.NewInt(0),
			Gas:      framework.DefaultGasLimit,
			Value:    big.NewInt(0),
			V:        big.NewInt(1), // it is necessary to encode in rlp,
			Input:    append(s.eventData.methodABI.ID(), encodedArgs...),
		}
	default:
		s.logger.Error("Unknown payload type")
	}

	return nil
}

// PopReadyTransaction removes the latest ready transaction from the SAMP
func (s *SAMUEL) PopReadyTransaction() {
	s.samp.Pop()
}

// getVerifiedSAMBucket returns the verified SAM bucket that
// has Quorum verified signatures
func (s *SAMUEL) getVerifiedSAMBucket(
	verifiedSAMs rootchain.VerifiedSAM,
) rootchain.VerifiedSAM {
	// Create the bucket map
	// childchainBlockNum -> verifiedSAMs
	samBuckets := make(map[uint64]rootchain.VerifiedSAM)

	// Sort the SAM messages into buckets
	for _, verifiedSAM := range verifiedSAMs {
		childBlockNum := verifiedSAM.ChildBlockNum

		// Check if there is already an aggregated array
		samArr, present := samBuckets[childBlockNum]
		if !present {
			samArr = make(rootchain.VerifiedSAM, 0)
		}

		samArr = append(samArr, verifiedSAM)
		samBuckets[childBlockNum] = samArr
	}

	var (
		chosenBucket     uint64 = 0
		candidateBuckets        = make([]uint64, 0)
	)

	// Get buckets that have Quorum signatures
	for blockNum, sams := range samBuckets {
		if uint64(len(sams)) >= s.signer.Quorum(blockNum) {
			candidateBuckets = append(candidateBuckets, blockNum)
		}
	}

	if len(candidateBuckets) == 0 {
		// No candidate SAMs
		return nil
	}

	// Out of all the candidate buckets, pick the one with the
	// lowest quorum threshold
	for _, bucketNumber := range candidateBuckets {
		if chosenBucket == 0 {
			// No bucket is chosen yet, assign it
			chosenBucket = bucketNumber

			continue
		}

		// Check if the current bucket has a lower quorum
		// threshold, and if so accept it
		if s.signer.Quorum(bucketNumber) <= s.signer.Quorum(chosenBucket) {
			chosenBucket = bucketNumber
		}
	}

	// Check if there is no Quorum
	// verified SAM array
	if chosenBucket == 0 {
		return nil
	}

	return samBuckets[chosenBucket]
}
