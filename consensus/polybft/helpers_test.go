package polybft

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func createTestKey(t *testing.T) *wallet.Key {
	t.Helper()

	return wallet.NewKey(wallet.GenerateAccount())
}

func createSignature(t *testing.T, accounts []*wallet.Account, hash types.Hash) *Signature {
	t.Helper()

	var signatures bls.Signatures

	var bmp bitmap.Bitmap
	for i, x := range accounts {
		bmp.Set(uint64(i))

		src, err := x.Bls.Sign(hash[:])
		require.NoError(t, err)

		signatures = append(signatures, src)
	}

	aggs, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)

	return &Signature{AggregatedSignature: aggs, Bitmap: bmp}
}

func generateStateSyncEvents(t *testing.T, eventsCount int, startIdx uint64) []*types.StateSyncEvent {
	t.Helper()

	stateSyncEvents := make([]*types.StateSyncEvent, eventsCount)
	for i := 0; i < eventsCount; i++ {
		stateSyncEvents[i] = &types.StateSyncEvent{
			ID:     startIdx + uint64(i),
			Sender: ethgo.Address(types.StringToAddress(fmt.Sprintf("0x5%d", i))),
			Data:   generateRandomBytes(t),
		}
	}

	return stateSyncEvents
}

// generateRandomBytes generates byte array with random data of 32 bytes length
func generateRandomBytes(t *testing.T) (result []byte) {
	t.Helper()

	result = make([]byte, types.HashLength)
	_, err := rand.Reader.Read(result)
	require.NoError(t, err, "Cannot generate random byte array content.")

	return
}

// getEpochNumber returns epoch number for given blockNumber and epochSize.
// Epoch number is derived as a result of division of block number and epoch size.
// Since epoch number is 1-based (0 block represents special case zero epoch),
// we are incrementing result by one for non epoch-ending blocks.
func getEpochNumber(t *testing.T, blockNumber, epochSize uint64) uint64 {
	t.Helper()

	if isEndOfPeriod(blockNumber, epochSize) {
		return blockNumber / epochSize
	}

	return blockNumber/epochSize + 1
}
