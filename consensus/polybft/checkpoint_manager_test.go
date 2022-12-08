package polybft

import (
	"errors"
	"math/big"
	"strconv"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestCheckpointManager_submitCheckpoint(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E"})
	validatorsMetadata := validators.getPublicIdentities()
	rootchainMock := new(dummyRootchainInteractor)
	rootchainMock.On("Call", mock.Anything, mock.Anything, mock.Anything).
		Return("1", error(nil)).
		Once()
	rootchainMock.On("GetPendingNonce", mock.Anything).
		Return(uint64(1), error(nil)).
		Once()
	rootchainMock.On("SendTransaction", mock.Anything, mock.Anything, mock.Anything).
		Return(&ethgo.Receipt{Status: uint64(types.ReceiptSuccess)}, error(nil)).
		Times(2)

	backendMock := new(polybftBackendMock)
	backendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validatorsMetadata)

	checkpoint := &CheckpointData{
		BlockRound:  1,
		EpochNumber: 4,
		EventRoot:   types.BytesToHash(generateRandomBytes(t)),
	}
	extra := createTestExtraObject(validatorsMetadata, validatorsMetadata, 3, 3, 3)
	extra.Checkpoint = checkpoint

	latestCheckpointHeader := &types.Header{
		Number:    4,
		ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...)}
	latestCheckpointHeader.ComputeHash()

	checkpoint1 := checkpoint.Copy()
	checkpoint1.EpochNumber = 1

	checkpoint2 := checkpoint.Copy()
	checkpoint2.EpochNumber = 2

	checkpoint3 := checkpoint.Copy()
	checkpoint3.EpochNumber = 3

	extra = createTestExtraObject(validatorsMetadata, validatorsMetadata, 4, 4, 4)
	extra.Checkpoint = checkpoint1
	extra3 := createTestExtraObject(validatorsMetadata, validatorsMetadata, 4, 4, 4)
	extra3.Checkpoint = checkpoint3

	headersMap := &testHeadersMap{}
	header1 := &types.Header{
		Number:    1,
		ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
	}
	header1.ComputeHash()

	header2 := header1.Copy()
	header2.Number = 2
	header2.ComputeHash()

	header3 := &types.Header{
		Number:    3,
		ExtraData: append(make([]byte, ExtraVanity), extra3.MarshalRLPTo(nil)...),
	}
	header3.ComputeHash()

	headersMap.addHeader(header1)
	headersMap.addHeader(header2)
	headersMap.addHeader(header3)

	// mock blockchain
	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	validatorAcc := validators.getValidator("A")
	c := &checkpointManager{
		signer:           wallet.NewEcdsaSigner(validatorAcc.Key()),
		rootchain:        rootchainMock,
		consensusBackend: backendMock,
		blockchain:       blockchainMock,
		logger:           hclog.NewNullLogger(),
	}

	err := c.submitCheckpoint(*latestCheckpointHeader, false)
	require.NoError(t, err)
	rootchainMock.AssertExpectations(t)
}

func TestCheckpointManager_abiEncodeCheckpointBlock(t *testing.T) {
	t.Parallel()

	const epochSize = uint64(10)

	currentValidators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D"})
	nextValidators := newTestValidatorsWithAliases([]string{"E", "F", "G", "H"})
	header := &types.Header{Number: 50}
	checkpoint := &CheckpointData{
		BlockRound:  1,
		EpochNumber: getEpochNumber(header.Number, epochSize),
		EventRoot:   types.BytesToHash(generateRandomBytes(t)),
	}

	proposalHash := generateRandomBytes(t)

	bmp := bitmap.Bitmap{}
	i := uint64(0)
	signature := &bls.Signature{}

	currentValidators.iterAcct(nil, func(v *testValidator) {
		signature = signature.Aggregate(v.mustSign(proposalHash))
		bmp.Set(i)
		i++
	})

	aggSignature, err := signature.Marshal()
	require.NoError(t, err)

	extra := &Extra{Checkpoint: checkpoint}
	extra.Committed = &Signature{
		AggregatedSignature: aggSignature,
		Bitmap:              bmp,
	}
	header.ExtraData = append(make([]byte, signer.IstanbulExtraVanity), extra.MarshalRLPTo(nil)...)
	header.ComputeHash()

	backendMock := new(polybftBackendMock)
	backendMock.On("GetValidators", mock.Anything, mock.Anything).Return(currentValidators.getPublicIdentities())

	c := &checkpointManager{
		blockchain:       &blockchainMock{},
		consensusBackend: backendMock,
		logger:           hclog.NewNullLogger(),
	}
	checkpointDataEncoded, err := c.abiEncodeCheckpointBlock(header.Number, header.Hash, *extra, nextValidators.getPublicIdentities())
	require.NoError(t, err)

	decodedCheckpointData, err := submitCheckpointMethod.Inputs.Decode(checkpointDataEncoded[4:])
	require.NoError(t, err)

	submitCheckpointInputData, ok := decodedCheckpointData.(map[string]interface{})
	require.True(t, ok)

	checkpointData, ok := submitCheckpointInputData["checkpoint"].(map[string]interface{})
	require.True(t, ok)

	checkpointMetadata, ok := submitCheckpointInputData["checkpointMetadata"].(map[string]interface{})
	require.True(t, ok)

	eventRoot, ok := checkpointData["eventRoot"].([types.HashLength]byte)
	require.True(t, ok)

	blockRound, ok := checkpointMetadata["blockRound"].(*big.Int)
	require.True(t, ok)

	epochNumber, ok := checkpointData["epochNumber"].(*big.Int)
	require.True(t, ok)

	blockNumber, ok := checkpointData["blockNumber"].(*big.Int)
	require.True(t, ok)

	require.Equal(t, new(big.Int).SetUint64(checkpoint.EpochNumber), epochNumber)
	require.Equal(t, new(big.Int).SetUint64(header.Number), blockNumber)
	require.Equal(t, checkpoint.EventRoot, types.BytesToHash(eventRoot[:]))
	require.Equal(t, new(big.Int).SetUint64(checkpoint.BlockRound), blockRound)
}

func TestCheckpointManager_getCurrentCheckpointID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		checkpointID string
		returnError  error
		errSubstring string
	}{
		{
			name:         "Happy path",
			checkpointID: "16",
			returnError:  error(nil),
			errSubstring: "",
		},
		{
			name:         "Rootchain call returns an error",
			checkpointID: "",
			returnError:  errors.New("internal error"),
			errSubstring: "failed to invoke currentCheckpointId function on the rootchain",
		},
		{
			name:         "Failed to parse return value from rootchain",
			checkpointID: "Hello World!",
			returnError:  error(nil),
			errSubstring: "failed to convert current checkpoint id",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			rootchainMock := new(dummyRootchainInteractor)
			rootchainMock.On("Call", mock.Anything, mock.Anything, mock.Anything).
				Return(c.checkpointID, c.returnError).
				Once()

			checkpointMgr := &checkpointManager{
				rootchain: rootchainMock,
				logger:    hclog.NewNullLogger(),
			}
			actualCheckpointID, err := checkpointMgr.getLatestCheckpointBlock()
			if c.errSubstring == "" {
				expectedCheckpointID, err := strconv.ParseUint(c.checkpointID, 0, 64)
				require.NoError(t, err)
				require.Equal(t, expectedCheckpointID, actualCheckpointID)
			} else {
				require.ErrorContains(t, err, c.errSubstring)
			}

			rootchainMock.AssertExpectations(t)
		})
	}
}

func TestCheckpointManager_isCheckpointBlock(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		blockNumber       uint64
		checkpointsOffset uint64
		isCheckpointBlock bool
	}{
		{
			name:              "Not checkpoint block",
			blockNumber:       3,
			checkpointsOffset: 6,
			isCheckpointBlock: false,
		},
		{
			name:              "Checkpoint block",
			blockNumber:       6,
			checkpointsOffset: 6,
			isCheckpointBlock: true,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			checkpointMgr := newCheckpointManager(wallet.NewEcdsaSigner(createTestKey(t)), c.checkpointsOffset, nil, nil, nil, hclog.NewNullLogger())
			require.Equal(t, c.isCheckpointBlock, checkpointMgr.isCheckpointBlock(c.blockNumber))
		})
	}
}

var _ rootchainInteractor = (*dummyRootchainInteractor)(nil)

type dummyRootchainInteractor struct {
	mock.Mock
}

func (d dummyRootchainInteractor) Call(from types.Address, to types.Address, input []byte) (string, error) {
	args := d.Called(from, to, input)

	return args.String(0), args.Error(1)
}

func (d dummyRootchainInteractor) SendTransaction(nonce uint64, transaction *ethgo.Transaction, signer ethgo.Key) (*ethgo.Receipt, error) {
	args := d.Called(nonce, transaction, signer)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

func (d dummyRootchainInteractor) GetPendingNonce(address types.Address) (uint64, error) {
	args := d.Called(address)

	return args.Get(0).(uint64), args.Error(1) //nolint:forcetypeassert
}
