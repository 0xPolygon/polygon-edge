package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

func TestCheckpointManager_abiEncodeCheckpointBlock(t *testing.T) {
	const epochSize = uint64(10)

	currentValidators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D"})
	nextValidators := newTestValidatorsWithAliases([]string{"E", "F", "G", "H"})
	header := &types.Header{Number: 50}
	checkpoint := &CheckpointData{
		BlockRound:  1,
		EpochNumber: getEpochNumber(header.Number, epochSize),
		EventRoot:   types.BytesToHash(generateRandomBytes(t)),
	}
	extra := &Extra{Checkpoint: checkpoint}
	checkpointManager := &checkpointManager{blockchain: &blockchainMock{}}

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

	extra.Committed = &Signature{
		AggregatedSignature: aggSignature,
		Bitmap:              bmp,
	}
	header.ExtraData = append(make([]byte, signer.IstanbulExtraVanity), extra.MarshalRLPTo(nil)...)
	header.ComputeHash()

	checkpointDataEncoded, err := checkpointManager.abiEncodeCheckpointBlock(*header, *extra, nextValidators.getPublicIdentities())
	require.NoError(t, err)

	decodedCheckpointData, err := submitCheckpointMethod.Inputs.Decode(checkpointDataEncoded[4:])
	require.NoError(t, err)

	checkpointDataMap, ok := decodedCheckpointData.(map[string]interface{})
	require.True(t, ok)

	eventRootDecoded, ok := checkpointDataMap["eventRoot"].([types.HashLength]byte)
	require.True(t, ok)
	require.Equal(t, new(big.Int).SetUint64(checkpoint.BlockRound), checkpointDataMap["blockRound"])
	require.Equal(t, new(big.Int).SetUint64(checkpoint.EpochNumber), checkpointDataMap["epochNumber"])
	require.Equal(t, checkpoint.EventRoot, types.BytesToHash(eventRootDecoded[:]))
}
