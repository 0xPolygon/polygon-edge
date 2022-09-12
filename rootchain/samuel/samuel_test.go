package samuel

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/rootchain/payload"
	"github.com/0xPolygon/polygon-edge/rootchain/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestSAMUEL_Start(t *testing.T) {
	t.Parallel()

	var (
		hasSubscribed        = false
		hasRegistered        = false
		startedBlock  uint64 = 0
		eventTracker         = mockEventTracker{
			subscribeFn: func() <-chan rootchain.Event {
				hasSubscribed = true

				ch := make(chan rootchain.Event)
				close(ch)

				return ch
			},
			startFn: func(blockNum uint64) error {
				startedBlock = blockNum
				return nil
			},
		}
		transport = mockTransport{
			subscribeFn: func(f func(sam *proto.SAM)) error {
				hasRegistered = true

				return nil
			},
		}
		storage = mockStorage{}
	)

	// Create the SAMUEL instance
	s := &SAMUEL{
		transport:    transport,
		storage:      storage,
		eventTracker: eventTracker,
	}

	// Make sure there were no errors in starting
	assert.NoError(t, s.Start())

	// Make sure the event subscription is active
	assert.True(t, hasSubscribed)

	// Make sure the gossip handler is registered
	assert.True(t, hasRegistered)

	// Make sure the start block is the latest block
	assert.Equal(t, rootchain.LatestRootchainBlockNumber, startedBlock)
}

func TestSAMUEL_GetStartBlockNumber_Predefined(t *testing.T) {
	t.Parallel()

	var (
		storedBlockNumber uint64 = 100
		storedEventIndex  uint64 = 1
		storage                  = mockStorage{
			readFn: func(_ string) (string, bool) {
				return fmt.Sprintf(
					"%d:%d",
					storedEventIndex,
					storedBlockNumber,
				), true
			},
		}
	)

	s := &SAMUEL{
		storage: storage,
	}

	// Get the start block number
	startBlock, err := s.getStartBlockNumber()

	assert.NoError(t, err)
	assert.Equal(t, storedBlockNumber, startBlock)
}

func TestSAMUEL_GetLatestStartBlock(t *testing.T) {
	t.Parallel()

	s := &SAMUEL{
		storage: mockStorage{},
	}

	// Get the start block number
	startBlock, err := s.getStartBlockNumber()

	assert.NoError(t, err)
	assert.Equal(t, rootchain.LatestRootchainBlockNumber, startBlock)
}

func TestSAMUEL_NewSamuel(t *testing.T) {
	t.Parallel()

	var (
		eventTracker = mockEventTracker{}
		samp         = mockSAMP{}
		signer       = mockSigner{}
		storage      = mockStorage{}
		transport    = mockTransport{}
		logger       = hclog.NewNullLogger()
		event        = &rootchain.ConfigEvent{
			EventABI:     "event GreetEmit()",
			MethodABI:    "[ { \"anonymous\": false, \"inputs\": [ { \"indexed\": false, \"internalType\": \"bytes\", \"name\": \"data\", \"type\": \"bytes\" } ], \"name\": \"StateReceived\", \"type\": \"event\" }, { \"inputs\": [ { \"internalType\": \"bytes\", \"name\": \"data\", \"type\": \"bytes\" } ], \"name\": \"onStateReceived\", \"outputs\": [], \"stateMutability\": \"nonpayable\", \"type\": \"function\" } ]",
			MethodName:   "setGreeting",
			LocalAddress: types.StringToAddress("123").String(),
		}
	)

	s := NewSamuel(
		event,
		logger,
		eventTracker,
		samp,
		signer,
		storage,
		transport,
	)

	assert.NotNil(t, s)

	assert.Equal(t, s.eventData.localAddress, types.StringToAddress(event.LocalAddress))
	assert.Equal(t, s.eventData.payloadType, rootchain.ValidatorSetPayloadType)
}

func TestSAMUEL_GetEventPayload(t *testing.T) {
	t.Parallel()

	// Create an example of a valid payload implementation
	vsPayload := payload.NewValidatorSetPayload([]payload.ValidatorSetInfo{
		{
			Address:      []byte("address"),
			BLSPublicKey: []byte("BLS public key"),
		},
	})
	vsPayloadMarshalled, err := vsPayload.Marshal()
	if err != nil {
		t.Fatalf("unable to marshal standard event type, %v", err)
	}

	testTable := []struct {
		name            string
		eventPayload    []byte
		payloadType     uint64
		expectedPayload rootchain.Payload
		expectedErr     error
	}{
		{
			"invalid payload type",
			[]byte{},
			math.MaxUint64,
			nil,
			errUnknownPayloadType,
		},
		{
			"Validator Set Payload type",
			vsPayloadMarshalled,
			uint64(rootchain.ValidatorSetPayloadType),
			vsPayload,
			nil,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			resPayload, payloadErr := getEventPayload(
				testCase.eventPayload,
				testCase.payloadType,
			)

			if testCase.expectedErr != nil {
				assert.ErrorIs(t, payloadErr, errUnknownPayloadType)
				assert.Nil(t, resPayload)
			} else {
				assert.NoError(t, payloadErr)
				assert.Equal(t, testCase.expectedPayload, resPayload)
			}
		})
	}
}

func TestSAMUEL_RegisterGossipHandler(t *testing.T) {
	t.Parallel()

	// Create an example of a valid payload implementation
	vsPayload := payload.NewValidatorSetPayload([]payload.ValidatorSetInfo{
		{
			Address:      []byte("address"),
			BLSPublicKey: []byte("BLS public key"),
		},
	})
	vsPayloadMarshalled, err := vsPayload.Marshal()
	if err != nil {
		t.Fatalf("unable to marshal standard event type, %v", err)
	}

	rootchainBlockNumber := uint64(100)
	event := &proto.Event{
		Index:       0,
		BlockNumber: rootchainBlockNumber,
		PayloadType: uint64(rootchain.ValidatorSetPayloadType),
		Payload:     vsPayloadMarshalled,
	}

	eventHash, err := (&rootchain.Event{
		Index:       event.Index,
		BlockNumber: event.BlockNumber,
		Payload:     vsPayload,
	}).Marshal()
	if err != nil {
		t.Fatalf("unable to hash rootchain event, %v", err)
	}

	var (
		signature                            = []byte("signature")
		childchainBlockNumber uint64         = 10
		addedSAM              *rootchain.SAM = nil
		sam                                  = &proto.SAM{
			Hash:                  crypto.Keccak256(eventHash),
			Signature:             signature,
			ChildchainBlockNumber: childchainBlockNumber,
			Event:                 event,
		}

		transport = mockTransport{
			subscribeFn: func(f func(sam *proto.SAM)) error {
				f(sam)

				return nil
			},
		}
		signer = mockSigner{
			verifySignatureFn: func(
				hash []byte,
				sig []byte,
				childchainBlockNum uint64,
			) error {
				if !bytes.Equal(sam.Hash, hash) ||
					!bytes.Equal(signature, sig) ||
					childchainBlockNumber != childchainBlockNum {
					return errors.New("invalid params")
				}

				return nil
			},
		}
		samp = mockSAMP{
			addMessageFn: func(sam rootchain.SAM) error {
				addedSAM = &sam

				return nil
			},
		}
	)

	// Create a new SAMUEL instance
	s := &SAMUEL{
		logger:    hclog.NewNullLogger(),
		samp:      samp,
		transport: transport,
		signer:    signer,
	}

	// Make sure there is no error in registering the handler
	assert.NoError(t, s.registerGossipHandler())

	// Check if the event was properly added
	if addedSAM == nil {
		t.Fatalf("SAM message was not saved to the SAMP")
	}

	// Make sure the added SAM is correct
	assert.Equal(t, types.BytesToHash(sam.Hash), addedSAM.Hash)
	assert.Equal(t, sam.Signature, addedSAM.Signature)

	assert.Equal(t, sam.Event.Index, addedSAM.Event.Index)
	assert.Equal(t, sam.Event.BlockNumber, addedSAM.Event.BlockNumber)

	payloadType, payloadData := addedSAM.Event.Get()

	assert.Equal(t, rootchain.ValidatorSetPayloadType, payloadType)
	assert.Equal(t, vsPayloadMarshalled, payloadData)
}

func TestSAMUEL_startEventLoop(t *testing.T) {
	t.Parallel()

	var (
		publishedProto *proto.SAM     = nil
		addedSAM       *rootchain.SAM = nil
		evCh                          = make(chan rootchain.Event)
		signature                     = []byte("signature")
		sigBlock                      = uint64(1)
		index                         = uint64(10)
		blockNum                      = uint64(10)
		event                         = rootchain.Event{
			Index:       index,
			BlockNumber: blockNum,
			Payload: payload.NewValidatorSetPayload(
				[]payload.ValidatorSetInfo{
					{
						Address:      []byte("random address"),
						BLSPublicKey: []byte("random pub key"),
					},
				},
			),
		}
		wg sync.WaitGroup

		eventTracker = mockEventTracker{
			subscribeFn: func() <-chan rootchain.Event {
				return evCh
			},
		}
		transport = mockTransport{
			publishFn: func(sam *proto.SAM) error {
				publishedProto = sam

				return nil
			},
		}
		samp = mockSAMP{
			addMessageFn: func(sam rootchain.SAM) error {
				addedSAM = &sam

				wg.Done()

				return nil
			},
		}
		signer = mockSigner{
			signFn: func(_ []byte) ([]byte, uint64, error) {
				return signature, sigBlock, nil
			},
		}
	)

	// Create a SAMUEL instance
	s := &SAMUEL{
		eventTracker: eventTracker,
		transport:    transport,
		samp:         samp,
		signer:       signer,
	}

	wg.Add(1)

	// Start the Event loop
	s.startEventLoop()

	// Send an event on the listener channel
	evCh <- event

	wg.Wait()
	close(evCh)

	// Make sure the SAM is correct
	if addedSAM == nil {
		t.Fatalf("SAM message was not added")
	}

	// Make sure the captured event was processed correctly
	assert.True(t, bytes.Equal(addedSAM.Signature, signature))
	assert.NotNil(t, publishedProto)
}

func TestSAMUEL_SaveProgress(t *testing.T) {
	// TODO
}

func TestSAMUEL_GetReadyTransaction(t *testing.T) {
	// TODO
}

func TestSAMUEL_PopReadyTransaction(t *testing.T) {
	t.Parallel()

	var (
		sampPopped = false

		samp = mockSAMP{
			popFn: func() rootchain.VerifiedSAM {
				sampPopped = true

				return nil
			},
		}
	)

	// Create a SAMUEL instance with the SAMP
	s := &SAMUEL{
		samp: samp,
	}

	s.PopReadyTransaction()

	// Make sure the SAMP has been popped
	assert.True(t, sampPopped)
}
