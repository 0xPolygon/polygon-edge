package polybft

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func createTestKey(t *testing.T) *wallet.Key {
	t.Helper()

	return wallet.NewKey(wallet.GenerateAccount(), bls.DomainCheckpointManager)
}

func createRandomTestKeys(t *testing.T, numberOfKeys int) []*wallet.Key {
	t.Helper()

	result := make([]*wallet.Key, numberOfKeys, numberOfKeys)

	for i := 0; i < numberOfKeys; i++ {
		result[i] = wallet.NewKey(wallet.GenerateAccount(), bls.DomainCheckpointManager)
	}

	return result
}

func createSignature(t *testing.T, accounts []*wallet.Account, hash types.Hash) *Signature {
	t.Helper()

	var signatures bls.Signatures

	var bmp bitmap.Bitmap
	for i, x := range accounts {
		bmp.Set(uint64(i))

		src, err := x.Bls.Sign(hash[:], bls.DomainCheckpointManager)
		require.NoError(t, err)

		signatures = append(signatures, src)
	}

	aggs, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)

	return &Signature{AggregatedSignature: aggs, Bitmap: bmp}
}

func createTestCommitEpochInput(t *testing.T, epochID uint64, validatorSet AccountSet, epochSize uint64) *contractsapi.CommitEpochFunction {
	t.Helper()

	if validatorSet == nil {
		validatorSet = newTestValidators(5).getPublicIdentities()
	}

	var startBlock uint64 = 0
	if epochID > 1 {
		startBlock = (epochID - 1) * epochSize
	}

	uptime := &contractsapi.Uptime{
		EpochID:     new(big.Int).SetUint64(epochID),
		UptimeData:  []*contractsapi.UptimeData{},
		TotalBlocks: new(big.Int).SetUint64(epochSize),
	}

	commitEpoch := &contractsapi.CommitEpochFunction{
		ID: uptime.EpochID,
		Epoch: &contractsapi.Epoch{
			StartBlock: new(big.Int).SetUint64(startBlock + 1),
			EndBlock:   new(big.Int).SetUint64(epochSize * epochID),
			EpochRoot:  types.Hash{},
		},
		Uptime: uptime,
	}

	for i := range validatorSet {
		uptime.AddValidatorUptime(validatorSet[i].Address, int64(epochSize))
	}

	return commitEpoch
}

func generateStateSyncEvents(t *testing.T, eventsCount int, startIdx uint64) []*contractsapi.StateSyncedEvent {
	t.Helper()

	stateSyncEvents := make([]*contractsapi.StateSyncedEvent, eventsCount)
	for i := 0; i < eventsCount; i++ {
		stateSyncEvents[i] = &contractsapi.StateSyncedEvent{
			ID:     big.NewInt(int64(startIdx + uint64(i))),
			Sender: types.StringToAddress(fmt.Sprintf("0x5%d", i)),
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

// newTestState creates new instance of state used by tests.
func newTestState(t *testing.T) *State {
	t.Helper()

	dir := fmt.Sprintf("/tmp/consensus-temp_%v", time.Now().Format(time.RFC3339Nano))
	err := os.Mkdir(dir, 0777)

	if err != nil {
		t.Fatal(err)
	}

	state, err := newState(path.Join(dir, "my.db"), hclog.NewNullLogger(), make(chan struct{}))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	})

	return state
}
