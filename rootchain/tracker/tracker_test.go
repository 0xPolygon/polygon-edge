package tracker

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/rootchain/payload"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/ethgo"
)

//nolint
const (
	ValidatorSetChangeEventABI = `event ValidatorSetChange(uint256 indexed index,tuple(address ecdsaAddress, bytes blsPublicKey)[] Validator,int8 indexed changeType)`
)

func initEventTracker(
	rootchainWS string,
	evenABI string,
	PayloadType rootchain.PayloadType,
	blockConfirmation uint64,
) (*EventTracker, error) {
	// Tracer
	trackerConfig := rootchain.ConfigEvent{
		EventABI:           evenABI,
		MethodABI:          "",
		LocalAddress:       "0x56B572e4eD62e421bA66EB14cB21e9BE04821D77",
		PayloadType:        PayloadType,
		BlockConfirmations: blockConfirmation,
	}

	return NewEventTracker(hclog.NewNullLogger(), &trackerConfig, rootchainWS)
}

func TestEventTracker_calculateRange(t *testing.T) {
	t.Parallel()

	evenTracker, err := initEventTracker(
		"0x56B572e4eD62e421bA66EB14cB21e9BE04821D77",
		ValidatorSetChangeEventABI,
		rootchain.ValidatorSetPayloadType,
		6,
	)

	assert.NoError(t, err)

	var mockHeader = &types.Header{
		Number: 1,
	}

	t.Run(
		"Not enough confirmations",
		func(t *testing.T) {
			t.Parallel()

			// Latest block number is 4,
			// required confirmation is 6
			// fromBlock and toBlock should be nill
			mockHeader.Number = 4
			evenTracker.setFromBlock(1)
			fromBlock, toBlock := evenTracker.calculateRange(mockHeader)
			assert.Nil(t, fromBlock)
			assert.Nil(t, toBlock)
		})

	t.Run(
		"Enough confirmations",
		func(t *testing.T) {
			t.Parallel()

			// Latest block number is 15,
			// required confirmation is 6
			// fromBlock should be 1
			// toBlock should be 9 (15-6)
			mockHeader.Number = 15
			evenTracker.setFromBlock(1)
			fromBlock, toBlock := evenTracker.calculateRange(mockHeader)
			assert.NotNil(t, fromBlock)
			assert.NotNil(t, toBlock)
			assert.Equal(t, *fromBlock, uint64(1))
			assert.Equal(t, *toBlock, uint64(9))
		})
	t.Run(
		"From block is LatestRootchainBlockNumber",
		func(t *testing.T) {
			t.Parallel()

			// Latest block number is 15,
			// required confirmation is 6
			// because fromBlock is set to be LatestRootchainBlockNumber
			// toBlock and fromBlock should be same, 9 (15-6)
			mockHeader.Number = 15
			evenTracker.setFromBlock(rootchain.LatestRootchainBlockNumber)
			fromBlock, toBlock := evenTracker.calculateRange(mockHeader)
			assert.NotNil(t, fromBlock)
			assert.NotNil(t, toBlock)
			assert.Equal(t, *fromBlock, uint64(9))
			assert.Equal(t, *toBlock, uint64(9))
		})
}

//nolint
func TestEventTracker_encodeEventFromLog(t *testing.T) {
	t.Parallel()

	evenTracker, err := initEventTracker(
		"0x56B572e4eD62e421bA66EB14cB21e9BE04821D77",
		ValidatorSetChangeEventABI,
		rootchain.ValidatorSetPayloadType,
		6,
	)

	assert.NoError(t, err)

	t.Run(
		"cannot parse event log, log does not match event",
		func(t *testing.T) {
			t.Parallel()

			// create invalid log obj
			log := ethgo.Log{}

			// try to encode log, should not parse
			event, err := evenTracker.encodeEventFromLog(&log)
			assert.ErrorContains(t, err, "cannot parse event log")
			assert.Equal(t, event, rootchain.Event{})
		})

	t.Run(
		"cannot parse event, no payloadType defined",
		func(t *testing.T) {

			// set payloadType
			evenTracker.payloadType = 1

			// create valid log obj
			log := ethgo.Log{
				Topics: []ethgo.Hash{
					ethgo.HexToHash("0x40caed2793e7227535ab6e11ab12326238bc3ea1e10a9be208132b373f2e919c"),
					ethgo.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
					ethgo.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000003"),
				},
				Data: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 192, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 6, 15, 186, 34, 194, 8, 230, 57, 41, 52, 86, 58, 190, 99, 27, 74, 71, 207, 54, 136, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 66, 48, 120, 54, 49, 54, 50, 54, 51, 54, 52, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 33, 2, 183, 229, 194, 133, 83, 113, 101, 88, 164, 173, 229, 24, 15, 221, 231, 108, 100, 106, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 66, 48, 120, 54, 49, 54, 50, 54, 51, 54, 52, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 195, 22, 245, 222, 246, 171, 178, 187, 251, 108, 92, 65, 47, 208, 77, 229, 174, 232, 188, 56, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 66, 48, 120, 54, 49, 54, 50, 54, 51, 54, 52, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 93, 214, 129, 83, 193, 67, 190, 177, 226, 88, 88, 219, 50, 66, 74, 238, 84, 55, 105, 194, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 66, 48, 120, 54, 49, 54, 50, 54, 51, 54, 52, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			}

			// try to encode log, should parse but no payloadType
			_, err := evenTracker.encodeEventFromLog(&log)
			assert.ErrorIs(t, err, errEventParseNoPayloadType)

		})

	t.Run(
		"successfully encode validatorSetChange event from log",
		func(t *testing.T) {

			// set payloadType
			evenTracker.payloadType = 0

			// create required event
			payload := payload.NewValidatorSetPayload(
				[]payload.ValidatorSetInfo{
					{
						Address:      types.StringToAddress("0x060fbA22C208E6392934563ABE631b4A47cF3688").Bytes(),
						BLSPublicKey: []byte("0x6162636400000000000000000000000000000000000000000000000000000000"),
					},
					{
						Address:      types.StringToAddress("0x2102B7e5c28553716558a4aDE5180fDDe76C646A").Bytes(),
						BLSPublicKey: []byte("0x6162636400000000000000000000000000000000000000000000000000000000"),
					},
					{
						Address:      types.StringToAddress("0xC316F5deF6abb2BBFB6C5c412FD04de5AEE8Bc38").Bytes(),
						BLSPublicKey: []byte("0x6162636400000000000000000000000000000000000000000000000000000000"),
					},
					{
						Address:      types.StringToAddress("0x5Dd68153c143BEb1E25858Db32424AEE543769C2").Bytes(),
						BLSPublicKey: []byte("0x6162636400000000000000000000000000000000000000000000000000000000"),
					},
				})

			wantedEvent := rootchain.Event{
				Index:       1,
				BlockNumber: 0,
				Payload:     payload,
			}

			// create valid log obj
			log := ethgo.Log{
				Topics: []ethgo.Hash{
					ethgo.HexToHash("0x40caed2793e7227535ab6e11ab12326238bc3ea1e10a9be208132b373f2e919c"),
					ethgo.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
					ethgo.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000003"),
				},
				Data: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 192, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 6, 15, 186, 34, 194, 8, 230, 57, 41, 52, 86, 58, 190, 99, 27, 74, 71, 207, 54, 136, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 66, 48, 120, 54, 49, 54, 50, 54, 51, 54, 52, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 33, 2, 183, 229, 194, 133, 83, 113, 101, 88, 164, 173, 229, 24, 15, 221, 231, 108, 100, 106, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 66, 48, 120, 54, 49, 54, 50, 54, 51, 54, 52, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 195, 22, 245, 222, 246, 171, 178, 187, 251, 108, 92, 65, 47, 208, 77, 229, 174, 232, 188, 56, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 66, 48, 120, 54, 49, 54, 50, 54, 51, 54, 52, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 93, 214, 129, 83, 193, 67, 190, 177, 226, 88, 88, 219, 50, 66, 74, 238, 84, 55, 105, 194, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 66, 48, 120, 54, 49, 54, 50, 54, 51, 54, 52, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			}

			// try to encode log, should succeed
			event, err := evenTracker.encodeEventFromLog(&log)
			assert.NoError(t, err)
			assert.Equal(t, event, wantedEvent)
		})
}
