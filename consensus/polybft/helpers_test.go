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

	return wallet.NewKey(generateTestAccount(t))
}

func createRandomTestKeys(t *testing.T, numberOfKeys int) []*wallet.Key {
	t.Helper()

	result := make([]*wallet.Key, numberOfKeys, numberOfKeys)

	for i := 0; i < numberOfKeys; i++ {
		result[i] = wallet.NewKey(generateTestAccount(t))
	}

	return result
}

func createSignature(t *testing.T, accounts []*wallet.Account, hash types.Hash, domain []byte) *Signature {
	t.Helper()

	var signatures bls.Signatures

	var bmp bitmap.Bitmap
	for i, x := range accounts {
		bmp.Set(uint64(i))

		src, err := x.Bls.Sign(hash[:], domain)
		require.NoError(t, err)

		signatures = append(signatures, src)
	}

	aggs, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)

	return &Signature{AggregatedSignature: aggs, Bitmap: bmp}
}

func createTestCommitEpochInput(t *testing.T, epochID uint64,
	epochSize uint64) *contractsapi.CommitEpochValidatorSetFn {
	t.Helper()

	var startBlock uint64 = 0
	if epochID > 1 {
		startBlock = (epochID - 1) * epochSize
	}

	commitEpoch := &contractsapi.CommitEpochValidatorSetFn{
		ID: new(big.Int).SetUint64(epochID),
		Epoch: &contractsapi.Epoch{
			StartBlock: new(big.Int).SetUint64(startBlock + 1),
			EndBlock:   new(big.Int).SetUint64(epochSize * epochID),
			EpochRoot:  types.Hash{},
		},
	}

	return commitEpoch
}

func createTestDistributeRewardsInput(t *testing.T, epochID uint64,
	validatorSet AccountSet, epochSize uint64) *contractsapi.DistributeRewardForRewardPoolFn {
	t.Helper()

	if validatorSet == nil {
		validatorSet = newTestValidators(t, 5).getPublicIdentities()
	}

	uptime := make([]*contractsapi.Uptime, len(validatorSet))

	for i, v := range validatorSet {
		uptime[i] = &contractsapi.Uptime{
			Validator:    v.Address,
			SignedBlocks: new(big.Int).SetUint64(epochSize),
		}
	}

	return &contractsapi.DistributeRewardForRewardPoolFn{
		EpochID: new(big.Int).SetUint64(epochID),
		Uptime:  uptime,
	}
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

	dir := fmt.Sprintf("/tmp/consensus-temp_%v", time.Now().UTC().Format(time.RFC3339Nano))
	err := os.Mkdir(dir, 0775)

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

func generateTestAccount(t *testing.T) *wallet.Account {
	t.Helper()

	acc, err := wallet.GenerateAccount()
	require.NoError(t, err)

	return acc
}

// createValidatorSetDelta calculates ValidatorSetDelta based on the provided old and new validator sets
func createValidatorSetDelta(oldValidatorSet, newValidatorSet AccountSet) (*ValidatorSetDelta, error) {
	var addedValidators, updatedValidators AccountSet

	oldValidatorSetMap := make(map[types.Address]*ValidatorMetadata)
	removedValidators := map[types.Address]int{}

	for i, validator := range oldValidatorSet {
		if (validator.Address != types.Address{}) {
			removedValidators[validator.Address] = i
			oldValidatorSetMap[validator.Address] = validator
		}
	}

	for _, newValidator := range newValidatorSet {
		// Check if the validator is among both old and new validator set
		oldValidator, validatorExists := oldValidatorSetMap[newValidator.Address]
		if validatorExists {
			if !oldValidator.EqualAddressAndBlsKey(newValidator) {
				return nil, fmt.Errorf("validator '%s' found in both old and new validator set, but its BLS keys differ",
					newValidator.Address.String())
			}

			// If it is, then discard it from removed validators...
			delete(removedValidators, newValidator.Address)

			if !oldValidator.Equals(newValidator) {
				updatedValidators = append(updatedValidators, newValidator)
			}
		} else {
			// ...otherwise it is added
			addedValidators = append(addedValidators, newValidator)
		}
	}

	removedValsBitmap := bitmap.Bitmap{}
	for _, i := range removedValidators {
		removedValsBitmap.Set(uint64(i))
	}

	delta := &ValidatorSetDelta{
		Added:   addedValidators,
		Updated: updatedValidators,
		Removed: removedValsBitmap,
	}

	return delta, nil
}
