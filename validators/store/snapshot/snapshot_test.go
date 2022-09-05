package snapshot

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	testHelper "github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

var (
	errTest = errors.New("test error")
)

// fakeValidator is a invalid validator
type fakeValidator struct {
	validators.Validator
}

func (f *fakeValidator) Addr() types.Address {
	return types.ZeroAddress
}

type mockSigner struct {
	TypeFn                func() validators.ValidatorType
	EcrecoverFromHeaderFn func(*types.Header) (types.Address, error)
	GetValidatorsFn       func(*types.Header) (validators.Validators, error)
}

func (m *mockSigner) Type() validators.ValidatorType {
	return m.TypeFn()
}

func (m *mockSigner) EcrecoverFromHeader(h *types.Header) (types.Address, error) {
	return m.EcrecoverFromHeaderFn(h)
}

func (m *mockSigner) GetValidators(h *types.Header) (validators.Validators, error) {
	return m.GetValidatorsFn(h)
}

func newTestHeaderHash(height uint64) types.Hash {
	return types.BytesToHash(crypto.Keccak256(big.NewInt(int64(height)).Bytes()))
}

func newTestHeader(height uint64, miner []byte, nonce types.Nonce) *types.Header {
	return &types.Header{
		Number: height,
		Hash:   newTestHeaderHash(height),
		Miner:  miner,
		Nonce:  nonce,
	}
}

func newMockBlockchain(latestHeight uint64, headers map[uint64]*types.Header) store.HeaderGetter {
	return &store.MockBlockchain{
		HeaderFn: func() *types.Header {
			return headers[latestHeight]
		},
		GetHeaderByNumberFn: func(height uint64) (*types.Header, bool) {
			header, ok := headers[height]

			return header, ok
		},
	}
}

func newTestSnapshotValidatorStore(
	blockchain store.HeaderGetter,
	getSigner func(uint64) (SignerInterface, error),
	lastBlock uint64,
	snapshots []*Snapshot,
	candidates []*store.Candidate,
	epochSize uint64,
) *SnapshotValidatorStore {
	return &SnapshotValidatorStore{
		logger: hclog.NewNullLogger(),
		store: newSnapshotStore(
			&SnapshotMetadata{
				LastBlock: lastBlock,
			},
			snapshots,
		),
		blockchain:     blockchain,
		getSigner:      getSigner,
		candidates:     candidates,
		candidatesLock: sync.RWMutex{},
		epochSize:      epochSize,
	}
}

func TestNewSnapshotValidatorStore(t *testing.T) {
	t.Parallel()

	var (
		logger     = hclog.NewNullLogger()
		blockchain = newMockBlockchain(
			0,
			map[uint64]*types.Header{
				0: newTestHeader(
					0,
					types.ZeroAddress.Bytes(),
					types.Nonce{},
				),
			},
		)

		epochSize uint64 = 10
		metadata         = &SnapshotMetadata{
			LastBlock: 20,
		}
		snapshots = []*Snapshot{}
	)

	t.Run("should return error", func(t *testing.T) {
		t.Parallel()

		store, err := NewSnapshotValidatorStore(
			logger,
			blockchain,
			func(u uint64) (SignerInterface, error) {
				return nil, errTest
			},
			epochSize,
			metadata,
			snapshots,
		)

		assert.Nil(
			t,
			store,
		)

		assert.Equal(
			t,
			errTest,
			err,
		)
	})

	t.Run("should succeed", func(t *testing.T) {
		t.Parallel()

		vals := validators.NewECDSAValidatorSet(
			ecdsaValidator1,
			ecdsaValidator2,
		)

		getSigner := func(u uint64) (SignerInterface, error) {
			return &mockSigner{
				GetValidatorsFn: func(h *types.Header) (validators.Validators, error) {
					return vals, nil
				},
			}, nil
		}

		snapshotStore, err := NewSnapshotValidatorStore(
			logger,
			blockchain,
			getSigner,
			epochSize,
			metadata,
			snapshots,
		)

		assert.Equal(
			t,
			snapshotStore.store,
			newSnapshotStore(metadata, []*Snapshot{
				{
					Number: 0,
					Hash:   newTestHeaderHash(0).String(),
					Set:    vals,
					Votes:  []*store.Vote{},
				},
			}),
		)

		assert.Equal(
			t,
			make([]*store.Candidate, 0),
			snapshotStore.candidates,
		)

		assert.Equal(
			t,
			snapshotStore.epochSize,
			epochSize,
		)

		assert.NoError(
			t,
			err,
		)
	})
}

func TestSnapshotValidatorStore_initialize(t *testing.T) {
	t.Parallel()

	var (
		initialCandidates        = []*store.Candidate{}
		epochSize         uint64 = 10
	)

	newGetSigner := func(
		validatorType validators.ValidatorType,
		headerCreators map[uint64]types.Address,
		headerValidators map[uint64]validators.Validators,
	) func(uint64) (SignerInterface, error) {
		return func(u uint64) (SignerInterface, error) {
			return &mockSigner{
				TypeFn: func() validators.ValidatorType {
					return validatorType
				},
				EcrecoverFromHeaderFn: func(h *types.Header) (types.Address, error) {
					creator, ok := headerCreators[h.Number]
					if !ok {
						return types.ZeroAddress, errTest
					}

					return creator, nil
				},
				GetValidatorsFn: func(h *types.Header) (validators.Validators, error) {
					validators, ok := headerValidators[h.Number]
					if !ok {
						return nil, errTest
					}

					return validators, nil
				},
			}, nil
		}
	}

	tests := []struct {
		name               string
		latestHeaderNumber uint64
		headers            map[uint64]*types.Header
		headerCreators     map[uint64]types.Address
		headerValidators   map[uint64]validators.Validators
		validatorType      validators.ValidatorType
		initialLastHeight  uint64
		initialSnapshots   []*Snapshot
		expectedErr        error
		finalLastHeight    uint64
		finalSnapshots     []*Snapshot
	}{
		{
			name:               "should a add snapshot created by genesis",
			latestHeaderNumber: 0,
			headers: map[uint64]*types.Header{
				0: newTestHeader(
					0,
					types.ZeroAddress.Bytes(),
					types.Nonce{},
				),
			},
			headerCreators: nil,
			headerValidators: map[uint64]validators.Validators{
				0: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
					ecdsaValidator2,
				),
			},
			validatorType:     validators.ECDSAValidatorType,
			initialLastHeight: 0,
			initialSnapshots:  []*Snapshot{},
			expectedErr:       nil,
			finalLastHeight:   0,
			finalSnapshots: []*Snapshot{
				{
					Number: 0,
					Hash:   newTestHeaderHash(0).String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{},
				},
			},
		},
		{
			name:               "should add a snapshot on the latest epoch if initial snapshots are empty",
			latestHeaderNumber: 20,
			headers: map[uint64]*types.Header{
				20: newTestHeader(
					20,
					types.ZeroAddress.Bytes(),
					types.Nonce{},
				),
			},
			headerCreators: nil,
			headerValidators: map[uint64]validators.Validators{
				20: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
					ecdsaValidator2,
				),
			},
			validatorType:     validators.ECDSAValidatorType,
			initialLastHeight: 10,
			initialSnapshots:  []*Snapshot{},
			expectedErr:       nil,
			finalLastHeight:   20,
			finalSnapshots: []*Snapshot{
				{
					Number: 20,
					Hash:   newTestHeaderHash(20).String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{},
				},
			},
		},
		{
			name:               "should add a snapshot on the latest epoch if the latest snapshot is not for the latest epoch",
			latestHeaderNumber: 20,
			headers: map[uint64]*types.Header{
				20: newTestHeader(
					20,
					types.ZeroAddress.Bytes(),
					types.Nonce{},
				),
			},
			headerCreators: nil,
			headerValidators: map[uint64]validators.Validators{
				20: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
					ecdsaValidator2,
				),
			},
			validatorType:     validators.ECDSAValidatorType,
			initialLastHeight: 10,
			initialSnapshots: []*Snapshot{
				{
					Number: 10,
					Hash:   newTestHeaderHash(10).String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{},
				},
			},
			expectedErr:     nil,
			finalLastHeight: 20,
			finalSnapshots: []*Snapshot{
				{
					Number: 10,
					Hash:   newTestHeaderHash(10).String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{},
				},
				{
					Number: 20,
					Hash:   newTestHeaderHash(20).String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{},
				},
			},
		},
		{
			name:               "should catch up latest header",
			latestHeaderNumber: 22,
			headers: map[uint64]*types.Header{
				20: newTestHeader(
					20,
					types.ZeroAddress.Bytes(),
					types.Nonce{},
				),
				21: newTestHeader(
					21,
					ecdsaValidator3.Address.Bytes(),
					nonceAuthVote,
				),
				22: newTestHeader(
					22,
					ecdsaValidator1.Address.Bytes(),
					nonceDropVote,
				),
			},
			headerCreators: map[uint64]types.Address{
				21: ecdsaValidator1.Address,
				22: ecdsaValidator2.Address,
			},
			headerValidators: map[uint64]validators.Validators{
				20: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
					ecdsaValidator2,
				),
			},
			validatorType:     validators.ECDSAValidatorType,
			initialLastHeight: 20,
			initialSnapshots: []*Snapshot{
				{
					Number: 20,
					Hash:   newTestHeaderHash(20).String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{},
				},
			},
			expectedErr:     nil,
			finalLastHeight: 22,
			finalSnapshots: []*Snapshot{
				{
					Number: 20,
					Hash:   newTestHeaderHash(20).String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{},
				},
				{
					Number: 21,
					Hash:   newTestHeaderHash(21).String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{
						{
							Candidate: ecdsaValidator3,
							Validator: ecdsaValidator1.Address,
							Authorize: true,
						},
					},
				},
				{
					Number: 22,
					Hash:   newTestHeaderHash(22).String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{
						{
							Candidate: ecdsaValidator3,
							Validator: ecdsaValidator1.Address,
							Authorize: true,
						},
						{
							Candidate: ecdsaValidator1,
							Validator: ecdsaValidator2.Address,
							Authorize: false,
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				newMockBlockchain(test.latestHeaderNumber, test.headers),
				newGetSigner(test.validatorType, test.headerCreators, test.headerValidators),
				test.initialLastHeight,
				test.initialSnapshots,
				initialCandidates,
				epochSize,
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				snapshotStore.initialize(),
			)

			assert.Equal(
				t,
				test.finalSnapshots,
				snapshotStore.GetSnapshots(),
			)
			assert.Equal(
				t,
				test.finalLastHeight,
				snapshotStore.GetSnapshotMetadata().LastBlock,
			)
			assert.Equal(
				t,
				initialCandidates,
				snapshotStore.candidates,
			)
		})
	}
}

func TestSnapshotValidatorStoreSourceType(t *testing.T) {
	t.Parallel()

	snapshotStore := newTestSnapshotValidatorStore(
		nil,
		nil,
		0,
		nil,
		nil,
		0,
	)

	assert.Equal(
		t,
		store.Snapshot,
		snapshotStore.SourceType(),
	)
}

func TestSnapshotValidatorStoreGetSnapshotMetadata(t *testing.T) {
	t.Parallel()

	var (
		lastBlock = uint64(10)

		snapshotStore = newTestSnapshotValidatorStore(
			nil,
			nil,
			lastBlock,
			nil,
			nil,
			0,
		)
	)

	assert.Equal(
		t,
		&SnapshotMetadata{
			LastBlock: lastBlock,
		},
		snapshotStore.GetSnapshotMetadata(),
	)
}

func TestSnapshotValidatorStoreGetSnapshots(t *testing.T) {
	t.Parallel()

	var (
		snapshots = []*Snapshot{
			{Number: 10},
			{Number: 20},
			{Number: 30},
		}

		snapshotStore = newTestSnapshotValidatorStore(
			nil,
			nil,
			0,
			snapshots,
			nil,
			0,
		)
	)

	assert.Equal(
		t,
		snapshots,
		snapshotStore.GetSnapshots(),
	)
}

func TestSnapshotValidatorStoreCandidates(t *testing.T) {
	t.Parallel()

	var (
		candidates = []*store.Candidate{
			{
				Validator: ecdsaValidator1,
				Authorize: true,
			},
		}

		snapshotStore = newTestSnapshotValidatorStore(
			nil,
			nil,
			0,
			nil,
			nil,
			0,
		)
	)

	snapshotStore.candidates = candidates

	assert.Equal(
		t,
		candidates,
		snapshotStore.Candidates(),
	)
}

func TestSnapshotValidatorStoreGetValidators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		snapshots   []*Snapshot
		height      uint64
		expectedRes validators.Validators
		expectedErr error
	}{
		{
			name:        "should return ErrSnapshotNotFound is the list is empty",
			snapshots:   []*Snapshot{},
			height:      10,
			expectedRes: nil,
			expectedErr: ErrSnapshotNotFound,
		},
		{
			name: "should return validators in the Snapshot for the given height",
			snapshots: []*Snapshot{
				{Number: 10},
				{
					Number: 20,
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
				},
				{Number: 30},
			},
			height: 25,
			expectedRes: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
				ecdsaValidator2,
			),
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				nil,
				0,
				test.snapshots,
				nil,
				0,
			)

			res, err := snapshotStore.GetValidatorsByHeight(test.height)

			assert.Equal(t, test.expectedRes, res)
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestSnapshotValidatorStoreVotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		snapshots   []*Snapshot
		height      uint64
		expectedRes []*store.Vote
		expectedErr error
	}{
		{
			name:        "should return ErrSnapshotNotFound is the list is empty",
			snapshots:   []*Snapshot{},
			height:      10,
			expectedRes: nil,
			expectedErr: ErrSnapshotNotFound,
		},
		{
			name: "should return validators in the Snapshot for the given height",
			snapshots: []*Snapshot{
				{Number: 10},
				{
					Number: 20,
					Votes: []*store.Vote{
						newTestVote(ecdsaValidator2, ecdsaValidator1.Address, true),
					},
				},
				{Number: 30},
			},
			height: 25,
			expectedRes: []*store.Vote{
				newTestVote(ecdsaValidator2, ecdsaValidator1.Address, true),
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				nil,
				0,
				test.snapshots,
				nil,
				0,
			)

			res, err := snapshotStore.Votes(test.height)

			assert.Equal(t, test.expectedRes, res)
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestSnapshotValidatorStoreUpdateValidatorSet(t *testing.T) {
	t.Parallel()

	var (
		// Add a snapshot so that snapshot can be used from the target height
		targetHeight uint64 = 21

		header = newTestHeader(targetHeight-1, nil, types.Nonce{})

		oldValidators = validators.NewECDSAValidatorSet(
			ecdsaValidator1,
		)

		newValidators = validators.NewECDSAValidatorSet(
			ecdsaValidator1,
			ecdsaValidator2,
			ecdsaValidator3,
		)
	)

	tests := []struct {
		name             string
		initialSnapshots []*Snapshot
		blockchain       *store.MockBlockchain
		// input
		newValidators validators.Validators
		height        uint64
		// output
		expectedErr    error
		finalSnapshots []*Snapshot
	}{
		{
			name: "should return an error if the blockchain doesn't have the header",
			initialSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
			},
			blockchain: &store.MockBlockchain{
				GetHeaderByNumberFn: func(height uint64) (*types.Header, bool) {
					assert.Equal(t, targetHeight-1, height)

					// not found
					return nil, false
				},
			},
			newValidators: nil,
			height:        targetHeight,
			expectedErr:   fmt.Errorf("header at %d not found", targetHeight-1),
			finalSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
			},
		},
		{
			name: "should replace a new snapshot with the snapshot that has the same height",
			initialSnapshots: []*Snapshot{
				{Number: 10},
				{
					Number: 20,
					Set:    oldValidators,
					Votes: []*store.Vote{
						newTestVote(ecdsaValidator2, ecdsaValidator2.Address, true),
						newTestVote(ecdsaValidator3, ecdsaValidator2.Address, true),
					},
				},
			},
			blockchain: &store.MockBlockchain{
				GetHeaderByNumberFn: func(height uint64) (*types.Header, bool) {
					assert.Equal(t, targetHeight-1, height)

					return header, true
				},
			},
			newValidators: newValidators,
			height:        targetHeight,
			expectedErr:   nil,
			finalSnapshots: []*Snapshot{
				{Number: 10},
				{
					Number: header.Number,
					Hash:   header.Hash.String(),
					Set:    newValidators,
					Votes:  []*store.Vote{},
				},
			},
		},
		{
			name: "should add a new snapshot when the snapshot with the same height doesn't exist",
			initialSnapshots: []*Snapshot{
				{Number: 10},
			},
			blockchain: &store.MockBlockchain{
				GetHeaderByNumberFn: func(height uint64) (*types.Header, bool) {
					assert.Equal(t, targetHeight-1, height)

					return header, true
				},
			},
			newValidators: newValidators,
			height:        targetHeight,
			expectedErr:   nil,
			finalSnapshots: []*Snapshot{
				{Number: 10},
				{
					Number: header.Number,
					Hash:   header.Hash.String(),
					Set:    newValidators,
					Votes:  []*store.Vote{},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				test.blockchain,
				nil,
				20,
				test.initialSnapshots,
				nil,
				0,
			)

			err := snapshotStore.UpdateValidatorSet(
				test.newValidators,
				test.height,
			)

			testHelper.AssertErrorMessageContains(t, test.expectedErr, err)
			assert.Equal(t, test.finalSnapshots, snapshotStore.GetSnapshots())
		})
	}
}

func TestSnapshotValidatorStoreModifyHeader(t *testing.T) {
	t.Parallel()

	var (
		targetNumber uint64 = 20
	)

	newInitialHeader := func() *types.Header {
		return newTestHeader(0, types.ZeroAddress.Bytes(), types.Nonce{})
	}

	tests := []struct {
		name              string
		initialSnapshots  []*Snapshot
		initialCandidates []*store.Candidate
		// input
		proposer types.Address
		// output
		expectedErr    error
		expectedHeader *types.Header
	}{
		{
			name:             "should return ErrSnapshotNotFound if the snapshot not found",
			initialSnapshots: []*Snapshot{},
			proposer:         addr1,
			expectedErr:      ErrSnapshotNotFound,
			expectedHeader:   newInitialHeader(),
		},
		{
			name: "should return validators.ErrInvalidValidatorType if the candidate is invalid",
			initialSnapshots: []*Snapshot{
				{
					Number: targetNumber - 1,
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
					),
				},
			},
			initialCandidates: []*store.Candidate{
				{
					Validator: &fakeValidator{},
					Authorize: true,
				},
			},
			proposer:    addr1,
			expectedErr: validators.ErrInvalidValidatorType,
			expectedHeader: newTestHeader(
				0,
				nil,
				types.Nonce{},
			),
		},
		{
			name: "should update miner and nonce for the addition",
			initialSnapshots: []*Snapshot{
				{
					Number: targetNumber - 1,
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
					),
				},
			},
			initialCandidates: []*store.Candidate{
				{
					Validator: ecdsaValidator2,
					Authorize: true,
				},
			},
			proposer:    addr1,
			expectedErr: nil,
			expectedHeader: newTestHeader(
				0,
				ecdsaValidator2.Address.Bytes(),
				nonceAuthVote,
			),
		},
		{
			name: "should update miner and nonce for the deletion",
			initialSnapshots: []*Snapshot{
				{
					Number: targetNumber - 1,
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
				},
			},
			initialCandidates: []*store.Candidate{
				{
					Validator: ecdsaValidator2,
					Authorize: false,
				},
			},
			proposer:    addr1,
			expectedErr: nil,
			expectedHeader: newTestHeader(
				0,
				ecdsaValidator2.Address.Bytes(),
				nonceDropVote,
			),
		},
		{
			name: "should ignore the candidate for the addition if the candidate is in the validator set already",
			initialSnapshots: []*Snapshot{
				{
					Number: targetNumber - 1,
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
				},
			},
			initialCandidates: []*store.Candidate{
				{
					Validator: ecdsaValidator2,
					Authorize: true,
				},
			},
			proposer:       addr1,
			expectedErr:    nil,
			expectedHeader: newInitialHeader(),
		},
		{
			name: "should ignore the candidate for the deletion if the candidate isn't in the validator set",
			initialSnapshots: []*Snapshot{
				{
					Number: targetNumber - 1,
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
					),
				},
			},
			initialCandidates: []*store.Candidate{
				{
					Validator: ecdsaValidator2,
					Authorize: false,
				},
			},
			proposer:       addr1,
			expectedErr:    nil,
			expectedHeader: newInitialHeader(),
		},
		{
			name: "should ignore the candidate if the candidate has been voted",
			initialSnapshots: []*Snapshot{
				{
					Number: targetNumber - 1,
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
					),
					Votes: []*store.Vote{
						newTestVote(ecdsaValidator2, ecdsaValidator1.Address, true),
					},
				},
			},
			initialCandidates: []*store.Candidate{
				{
					Validator: ecdsaValidator2,
					Authorize: true,
				},
			},
			proposer:       addr1,
			expectedErr:    nil,
			expectedHeader: newInitialHeader(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			header := newInitialHeader()

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				nil,
				20,
				test.initialSnapshots,
				test.initialCandidates,
				0,
			)

			assert.Equal(
				t,
				test.expectedErr,
				snapshotStore.ModifyHeader(
					header,
					test.proposer,
				),
			)

			assert.Equal(t, test.expectedHeader, header)
		})
	}
}

func TestSnapshotValidatorStoreVerifyHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		nonce       types.Nonce
		expectedErr error
	}{
		{
			name: "should return nil in case of zero value nonce",
			// same with nonceDropVote
			nonce:       types.Nonce{},
			expectedErr: nil,
		},
		{
			name:        "should return nil in case of nonceAuthVote",
			nonce:       nonceAuthVote,
			expectedErr: nil,
		},
		{
			name:        "should return nil in case of nonceDropVote",
			nonce:       nonceDropVote,
			expectedErr: nil,
		},
		{
			name:        "should return ErrInvalidNonce in case of other nonces",
			nonce:       types.Nonce{0xff, 0x00},
			expectedErr: ErrInvalidNonce,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			header := &types.Header{
				Nonce: test.nonce,
			}

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				nil,
				20,
				nil,
				nil,
				0,
			)

			assert.Equal(
				t,
				test.expectedErr,
				snapshotStore.VerifyHeader(header),
			)
		})
	}
}

func TestSnapshotValidatorStoreProcessHeadersInRange(t *testing.T) {
	t.Parallel()

	var (
		epochSize     uint64 = 10
		validatorType        = validators.ECDSAValidatorType

		initialValidators = validators.NewECDSAValidatorSet(
			ecdsaValidator1,
		)
		initialLastHeight uint64 = 0
		initialSnapshot          = &Snapshot{
			Number: initialLastHeight,
			Set:    initialValidators,
			Votes:  []*store.Vote{},
		}
		initialSnapshots = []*Snapshot{
			initialSnapshot,
		}
		initialCandidates = []*store.Candidate{}
	)

	createHeaderWithVote := func(height uint64, candidate validators.Validator, nonce types.Nonce) *types.Header {
		candidateBytes, _ := validatorToMiner(candidate)

		return &types.Header{
			Number: height,
			Miner:  candidateBytes,
			Nonce:  nonce,
		}
	}

	tests := []struct {
		name            string
		from            uint64
		to              uint64
		headers         map[uint64]*types.Header
		headerCreators  map[uint64]types.Address
		expectedErr     error
		finalSnapshots  []*Snapshot
		finalLastHeight uint64
	}{
		{
			name:            "should return error if header not found",
			from:            0,
			to:              5,
			headers:         map[uint64]*types.Header{},
			headerCreators:  map[uint64]types.Address{},
			expectedErr:     fmt.Errorf("header %d not found", 1),
			finalSnapshots:  initialSnapshots,
			finalLastHeight: initialLastHeight,
		},
		{
			name: "should return error if ProcessHeader fails",
			from: 0,
			to:   2,
			headers: map[uint64]*types.Header{
				1: createHeaderWithVote(1, ecdsaValidator2, nonceAuthVote),
				2: createHeaderWithVote(2, ecdsaValidator2, nonceAuthVote),
			},
			headerCreators: map[uint64]types.Address{
				1: ecdsaValidator1.Address,
			},
			expectedErr: errTest,
			finalSnapshots: []*Snapshot{
				initialSnapshot,
				{
					Number: 1,
					Hash:   types.ZeroHash.String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{},
				},
			},
			finalLastHeight: 1,
		},
		{
			name: "should process all headers for ECDSAValidators",
			from: 1,
			to:   6,
			headers: map[uint64]*types.Header{
				1: createHeaderWithVote(1, ecdsaValidator2, nonceAuthVote),
				2: createHeaderWithVote(2, ecdsaValidator3, nonceAuthVote),
				3: createHeaderWithVote(3, ecdsaValidator3, nonceAuthVote),
				4: createHeaderWithVote(4, ecdsaValidator2, nonceDropVote),
				5: createHeaderWithVote(5, ecdsaValidator1, nonceDropVote),
				6: createHeaderWithVote(6, ecdsaValidator1, nonceDropVote),
			},
			headerCreators: map[uint64]types.Address{
				1: ecdsaValidator1.Address,
				2: ecdsaValidator2.Address,
				3: ecdsaValidator1.Address,
				4: ecdsaValidator1.Address,
				5: ecdsaValidator2.Address,
				6: ecdsaValidator3.Address,
			},
			expectedErr: nil,
			finalSnapshots: []*Snapshot{
				initialSnapshot,
				{
					Number: 1,
					Hash:   types.ZeroHash.String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{},
				},
				{
					Number: 2,
					Hash:   types.ZeroHash.String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{
						{
							Candidate: ecdsaValidator3,
							Validator: ecdsaValidator2.Address,
							Authorize: true,
						},
					},
				},
				{
					Number: 3,
					Hash:   types.ZeroHash.String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
						ecdsaValidator3,
					),
					Votes: []*store.Vote{},
				},
				{
					Number: 4,
					Hash:   types.ZeroHash.String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
						ecdsaValidator3,
					),
					Votes: []*store.Vote{
						{
							Candidate: ecdsaValidator2,
							Validator: ecdsaValidator1.Address,
							Authorize: false,
						},
					},
				},
				{
					Number: 5,
					Hash:   types.ZeroHash.String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
						ecdsaValidator3,
					),
					Votes: []*store.Vote{
						{
							Candidate: ecdsaValidator2,
							Validator: ecdsaValidator1.Address,
							Authorize: false,
						},
						{
							Candidate: ecdsaValidator1,
							Validator: ecdsaValidator2.Address,
							Authorize: false,
						},
					},
				},
				{
					Number: 6,
					Hash:   types.ZeroHash.String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator2,
						ecdsaValidator3,
					),
					Votes: []*store.Vote{},
				},
			},
			finalLastHeight: 6,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				newMockBlockchain(0, test.headers),
				func(u uint64) (SignerInterface, error) {
					return &mockSigner{
						TypeFn: func() validators.ValidatorType {
							return validatorType
						},
						EcrecoverFromHeaderFn: func(h *types.Header) (types.Address, error) {
							creator, ok := test.headerCreators[h.Number]
							if !ok {
								return types.ZeroAddress, errTest
							}

							return creator, nil
						},
					}, nil
				},
				initialLastHeight,
				initialSnapshots,
				initialCandidates,
				epochSize,
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				snapshotStore.ProcessHeadersInRange(test.from, test.to),
			)

			assert.Equal(
				t,
				test.finalSnapshots,
				snapshotStore.GetSnapshots(),
			)
			assert.Equal(
				t,
				test.finalLastHeight,
				snapshotStore.GetSnapshotMetadata().LastBlock,
			)
		})
	}
}

func TestSnapshotValidatorStoreProcessHeader(t *testing.T) {
	t.Parallel()

	var (
		epochSize         uint64 = 10
		initialLastHeight uint64 = 49
		headerHeight1     uint64 = 50
		headerHeight2     uint64 = 51

		initialValidators = validators.NewECDSAValidatorSet(
			ecdsaValidator1,
			ecdsaValidator2,
		)
		initialCandidates = []*store.Candidate{}
		initialSnapshot   = &Snapshot{
			Number: initialLastHeight,
			Set:    initialValidators,
		}
	)

	newGetSigner := func(
		validatorType validators.ValidatorType,
		expectedHeight uint64,
		expectedHeader *types.Header,
		returnAddress types.Address,
		returnError error,
	) func(uint64) (SignerInterface, error) {
		t.Helper()

		return func(height uint64) (SignerInterface, error) {
			assert.Equal(t, expectedHeight, height)

			return &mockSigner{
				TypeFn: func() validators.ValidatorType {
					return validatorType
				},
				EcrecoverFromHeaderFn: func(header *types.Header) (types.Address, error) {
					assert.Equal(t, expectedHeader, header)

					return returnAddress, returnError
				},
			}, nil
		}
	}

	tests := []struct {
		name             string
		getSigner        func(uint64) (SignerInterface, error)
		initialSnapshots []*Snapshot
		header           *types.Header
		expectedErr      error
		finalSnapshots   []*Snapshot
		finalLastBlock   uint64
	}{
		{
			name: "should return error if getSigner returns error",
			getSigner: func(height uint64) (SignerInterface, error) {
				assert.Equal(t, headerHeight1, height)

				return nil, errTest
			},
			initialSnapshots: []*Snapshot{},
			header: &types.Header{
				Number: headerHeight1,
			},
			expectedErr:    errTest,
			finalSnapshots: []*Snapshot{},
			finalLastBlock: initialLastHeight,
		},
		{
			name: "should return error if the signer is nil",
			getSigner: func(height uint64) (SignerInterface, error) {
				assert.Equal(t, headerHeight1, height)

				return nil, nil
			},
			initialSnapshots: []*Snapshot{},
			header: &types.Header{
				Number: headerHeight1,
			},
			expectedErr:    fmt.Errorf("signer not found at %d", headerHeight1),
			finalSnapshots: []*Snapshot{},
			finalLastBlock: initialLastHeight,
		},
		{
			name: "should return error if EcrecoverFromHeader fails",
			getSigner: newGetSigner(
				validators.ECDSAValidatorType,
				headerHeight1,
				&types.Header{Number: headerHeight1},
				types.ZeroAddress,
				errTest,
			),
			initialSnapshots: []*Snapshot{},
			header: &types.Header{
				Number: headerHeight1,
			},
			expectedErr:    errTest,
			finalSnapshots: []*Snapshot{},
			finalLastBlock: initialLastHeight,
		},
		{
			name: "should return error if snapshot not found",
			getSigner: newGetSigner(
				validators.ECDSAValidatorType,
				headerHeight1,
				&types.Header{Number: headerHeight1},
				ecdsaValidator3.Address,
				nil,
			),
			initialSnapshots: []*Snapshot{},
			header: &types.Header{
				Number: headerHeight1,
			},
			expectedErr:    ErrSnapshotNotFound,
			finalSnapshots: []*Snapshot{},
			finalLastBlock: initialLastHeight,
		},
		{
			name: "should return ErrUnauthorizedProposer if the header creator is not the validator in the snapshot",
			getSigner: newGetSigner(
				validators.ECDSAValidatorType,
				headerHeight1,
				&types.Header{Number: headerHeight1},
				ecdsaValidator3.Address,
				nil,
			),
			initialSnapshots: []*Snapshot{
				initialSnapshot,
			},
			header: &types.Header{
				Number: headerHeight1,
			},
			expectedErr: ErrUnauthorizedProposer,
			finalSnapshots: []*Snapshot{
				initialSnapshot,
			},
			finalLastBlock: initialLastHeight,
		},
		{
			name: "should reset votes and remove lower snapshots if the height is the beginning of the epoch",
			getSigner: newGetSigner(
				validators.ECDSAValidatorType, headerHeight1,
				&types.Header{Number: headerHeight1},
				ecdsaValidator1.Address, nil,
			),
			initialSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
				{Number: 30},
				{Number: 40},
				{
					Number: initialLastHeight,
					Set:    initialValidators,
					Votes: []*store.Vote{
						newTestVote(ecdsaValidator3, ecdsaValidator1.Address, true),
						newTestVote(ecdsaValidator1, ecdsaValidator2.Address, false),
					},
				},
			},
			header: &types.Header{
				Number: headerHeight1,
			},
			expectedErr: nil,
			finalSnapshots: []*Snapshot{
				{Number: 30},
				{Number: 40},
				{
					Number: initialLastHeight,
					Set:    initialValidators,
					Votes: []*store.Vote{
						newTestVote(ecdsaValidator3, ecdsaValidator1.Address, true),
						newTestVote(ecdsaValidator1, ecdsaValidator2.Address, false),
					},
				},
				{
					Number: headerHeight1,
					Hash:   types.ZeroHash.String(),
					Set:    initialValidators,
				},
			},
			finalLastBlock: headerHeight1,
		},
		{
			name: "should just update latest height if miner is zero",
			getSigner: newGetSigner(
				validators.ECDSAValidatorType,
				headerHeight2, &types.Header{Number: headerHeight2, Miner: types.ZeroAddress.Bytes()},
				ecdsaValidator1.Address, nil,
			),
			initialSnapshots: []*Snapshot{
				{
					Number: initialLastHeight,
					Set:    initialValidators,
					Votes: []*store.Vote{
						newTestVote(ecdsaValidator3, ecdsaValidator1.Address, true),
						newTestVote(ecdsaValidator1, ecdsaValidator2.Address, false),
					},
				},
			},
			header: &types.Header{
				Number: headerHeight2,
				Miner:  types.ZeroAddress.Bytes(),
			},
			expectedErr: nil,
			finalSnapshots: []*Snapshot{
				{
					Number: initialLastHeight,
					Set:    initialValidators,
					Votes: []*store.Vote{
						newTestVote(ecdsaValidator3, ecdsaValidator1.Address, true),
						newTestVote(ecdsaValidator1, ecdsaValidator2.Address, false),
					},
				},
			},
			finalLastBlock: headerHeight2,
		},
		{
			name: "should process the vote in the header and update snapshots and latest height",
			getSigner: newGetSigner(
				validators.ECDSAValidatorType, headerHeight2, &types.Header{
					Number: headerHeight2,
					Miner:  ecdsaValidator3.Address.Bytes(),
					Nonce:  nonceAuthVote,
				}, ecdsaValidator1.Address, nil,
			),
			initialSnapshots: []*Snapshot{
				{
					Number: initialLastHeight,
					Set:    initialValidators,
					Votes: []*store.Vote{
						newTestVote(ecdsaValidator3, ecdsaValidator2.Address, true),
					},
				},
			},
			header: &types.Header{
				Number: headerHeight2,
				Miner:  ecdsaValidator3.Address.Bytes(),
				Nonce:  nonceAuthVote,
			},
			expectedErr: nil,
			finalSnapshots: []*Snapshot{
				{
					Number: initialLastHeight,
					Set:    initialValidators,
					Votes: []*store.Vote{
						newTestVote(ecdsaValidator3, ecdsaValidator2.Address, true),
					},
				},
				{
					Number: headerHeight2,
					Hash:   types.ZeroHash.String(),
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
						ecdsaValidator3,
					),
					Votes: []*store.Vote{},
				},
			},
			finalLastBlock: headerHeight2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				test.getSigner,
				initialLastHeight,
				test.initialSnapshots,
				initialCandidates,
				epochSize,
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				snapshotStore.ProcessHeader(
					test.header,
				),
			)

			assert.Equal(
				t,
				test.finalSnapshots,
				snapshotStore.GetSnapshots(),
			)

			assert.Equal(
				t,
				test.finalLastBlock,
				snapshotStore.GetSnapshotMetadata().LastBlock,
			)
		})
	}
}

func TestSnapshotValidatorStorePropose(t *testing.T) {
	t.Parallel()

	var (
		latestHeight uint64 = 20
	)

	tests := []struct {
		name              string
		initialSnapshots  []*Snapshot
		initialCandidates []*store.Candidate
		candidate         validators.Validator
		auth              bool
		proposer          types.Address
		expectedErr       error
		finalCandidates   []*store.Candidate
	}{
		{
			name:             "should return ErrAlreadyCandidate if the candidate exists in the candidates already",
			initialSnapshots: nil,
			initialCandidates: []*store.Candidate{
				{
					Validator: ecdsaValidator2,
					Authorize: true,
				},
			},
			candidate:   ecdsaValidator2,
			auth:        true,
			proposer:    ecdsaValidator1.Address,
			expectedErr: ErrAlreadyCandidate,
			finalCandidates: []*store.Candidate{
				{
					Validator: ecdsaValidator2,
					Authorize: true,
				},
			},
		},
		{
			name:              "should return ErrSnapshotNotFound if snapshot not found",
			initialSnapshots:  []*Snapshot{},
			initialCandidates: []*store.Candidate{},
			candidate:         ecdsaValidator2,
			auth:              true,
			proposer:          ecdsaValidator1.Address,
			expectedErr:       ErrSnapshotNotFound,
			finalCandidates:   []*store.Candidate{},
		},
		{
			name: "should return ErrCandidateIsValidator if the candidate for addition exists in the validator set already",
			initialSnapshots: []*Snapshot{
				{
					Number: latestHeight,
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
				},
			},
			initialCandidates: []*store.Candidate{},
			candidate:         ecdsaValidator2,
			auth:              true,
			proposer:          ecdsaValidator1.Address,
			expectedErr:       ErrCandidateIsValidator,
			finalCandidates:   []*store.Candidate{},
		},
		{
			name: "should return ErrCandidateNotExistInSet if the candidate for deletion doesn't exist in the validator set",
			initialSnapshots: []*Snapshot{
				{
					Number: latestHeight,
					Set: validators.NewBLSValidatorSet(
						blsValidator1,
						blsValidator2,
					),
				},
			},
			initialCandidates: []*store.Candidate{},
			candidate:         blsValidator3,
			auth:              false,
			proposer:          blsValidator1.Address,
			expectedErr:       ErrCandidateNotExistInSet,
			finalCandidates:   []*store.Candidate{},
		},
		{
			name: "should return ErrAlreadyVoted if the proposer has voted for the same candidate",
			initialSnapshots: []*Snapshot{
				{
					Number: latestHeight,
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{
						newTestVote(ecdsaValidator3, ecdsaValidator1.Address, true),
					},
				},
			},
			initialCandidates: []*store.Candidate{},
			candidate:         ecdsaValidator3,
			auth:              true,
			proposer:          ecdsaValidator1.Address,
			expectedErr:       ErrAlreadyVoted,
			finalCandidates:   []*store.Candidate{},
		},
		{
			name: "should add a new candidate",
			initialSnapshots: []*Snapshot{
				{
					Number: latestHeight,
					Set: validators.NewECDSAValidatorSet(
						ecdsaValidator1,
						ecdsaValidator2,
					),
					Votes: []*store.Vote{},
				},
			},
			initialCandidates: []*store.Candidate{},
			candidate:         ecdsaValidator3,
			auth:              true,
			proposer:          ecdsaValidator1.Address,
			expectedErr:       nil,
			finalCandidates: []*store.Candidate{
				{
					Validator: ecdsaValidator3,
					Authorize: true,
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				nil,
				latestHeight,
				test.initialSnapshots,
				test.initialCandidates,
				0,
			)

			assert.Equal(
				t,
				test.expectedErr,
				snapshotStore.Propose(
					test.candidate,
					test.auth,
					test.proposer,
				),
			)
		})
	}
}

func TestSnapshotValidatorStore_addCandidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		initialCandidates []*store.Candidate
		// input
		validators validators.Validators
		candidate  validators.Validator
		authorize  bool
		// output
		expectedErr     error
		finalCandidates []*store.Candidate
	}{
		{
			name:              "should add a new candidate for addition",
			initialCandidates: []*store.Candidate{},
			validators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
			),
			candidate:   ecdsaValidator2,
			authorize:   true,
			expectedErr: nil,
			finalCandidates: []*store.Candidate{
				{
					Validator: ecdsaValidator2,
					Authorize: true,
				},
			},
		},
		{
			name:              "should return ErrCandidateNotExistInSet if the candidate to be removed doesn't exist in set",
			initialCandidates: []*store.Candidate{},
			validators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
			),
			candidate:       ecdsaValidator2,
			authorize:       false,
			expectedErr:     ErrCandidateNotExistInSet,
			finalCandidates: []*store.Candidate{},
		},
		{
			name:              "should add a new candidate for deletion",
			initialCandidates: []*store.Candidate{},
			validators: validators.NewBLSValidatorSet(
				blsValidator1,
				blsValidator2,
			),
			// candidate just has to have the Address field only
			candidate:   validators.NewBLSValidator(blsValidator2.Addr(), nil),
			authorize:   false,
			expectedErr: nil,
			finalCandidates: []*store.Candidate{
				{
					Validator: blsValidator2,
					Authorize: false,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				nil,
				20,
				nil,
				test.initialCandidates,
				0,
			)

			assert.Equal(
				t,
				test.expectedErr,
				snapshotStore.addCandidate(
					test.validators,
					test.candidate,
					test.authorize,
				),
			)

			assert.Equal(
				t,
				test.finalCandidates,
				snapshotStore.candidates,
			)
		})
	}
}

func TestSnapshotValidatorStore_addHeaderSnap(t *testing.T) {
	t.Parallel()

	var (
		headerHeight uint64 = 10
		headerHash          = types.BytesToHash(crypto.Keccak256([]byte{byte(headerHeight)}))
		header              = &types.Header{
			Number: headerHeight,
			Hash:   headerHash,
		}

		newValidators = validators.NewECDSAValidatorSet(
			ecdsaValidator1,
			ecdsaValidator2,
		)
	)

	tests := []struct {
		name             string
		getSigner        func(uint64) (SignerInterface, error)
		initialSnapshots []*Snapshot
		header           *types.Header
		finalSnapshots   []*Snapshot
		expectedErr      error
	}{
		{
			name: "should return error if getSigner fails",
			getSigner: func(height uint64) (SignerInterface, error) {
				assert.Equal(t, headerHeight, height)

				return nil, errTest
			},
			initialSnapshots: []*Snapshot{},
			header:           header,
			expectedErr:      errTest,
			finalSnapshots:   []*Snapshot{},
		},
		{
			name: "should return error if getSigner returns nil",
			getSigner: func(height uint64) (SignerInterface, error) {
				assert.Equal(t, headerHeight, height)

				return nil, nil
			},
			initialSnapshots: []*Snapshot{},
			header:           header,
			expectedErr:      fmt.Errorf("signer not found %d", headerHeight),
			finalSnapshots:   []*Snapshot{},
		},
		{
			name: "should return error if signer.GetValidators fails",
			getSigner: func(height uint64) (SignerInterface, error) {
				assert.Equal(t, headerHeight, height)

				return &mockSigner{
					GetValidatorsFn: func(h *types.Header) (validators.Validators, error) {
						return nil, errTest
					},
				}, nil
			},
			initialSnapshots: []*Snapshot{},
			header:           header,
			expectedErr:      errTest,
			finalSnapshots:   []*Snapshot{},
		},
		{
			name: "should add a new snapshot",
			getSigner: func(height uint64) (SignerInterface, error) {
				assert.Equal(t, headerHeight, height)

				return &mockSigner{
					GetValidatorsFn: func(h *types.Header) (validators.Validators, error) {
						return newValidators, nil
					},
				}, nil
			},
			initialSnapshots: []*Snapshot{},
			header:           header,
			expectedErr:      nil,
			finalSnapshots: []*Snapshot{
				{
					Number: headerHeight,
					Hash:   headerHash.String(),
					Votes:  []*store.Vote{},
					Set:    newValidators,
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				test.getSigner,
				20,
				test.initialSnapshots,
				nil,
				0,
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				snapshotStore.addHeaderSnap(
					test.header,
				),
			)

			assert.Equal(
				t,
				test.finalSnapshots,
				snapshotStore.GetSnapshots(),
			)
		})
	}
}

func TestSnapshotValidatorStore_getSnapshot(t *testing.T) {
	t.Parallel()

	var (
		snapthots = []*Snapshot{
			{Number: 10},
			{Number: 20},
			{Number: 30},
		}

		expectedSnapshot = &Snapshot{
			Number: 20,
		}

		targetHeight uint64 = 20
	)

	snapshotStore := newTestSnapshotValidatorStore(
		nil,
		nil,
		20,
		snapthots,
		nil,
		0,
	)

	assert.Equal(
		t,
		expectedSnapshot,
		snapshotStore.getSnapshot(targetHeight),
	)
}

func TestSnapshotValidatorStore_getLatestSnapshot(t *testing.T) {
	t.Parallel()

	var (
		snapthots = []*Snapshot{
			{Number: 10},
			{Number: 20},
			{Number: 30},
		}

		expectedSnapshot = &Snapshot{
			Number: 10,
		}

		latestHeight uint64 = 11
	)

	snapshotStore := newTestSnapshotValidatorStore(
		nil,
		nil,
		latestHeight,
		snapthots,
		nil,
		0,
	)

	assert.Equal(
		t,
		expectedSnapshot,
		snapshotStore.getLatestSnapshot(),
	)
}

func TestSnapshotValidatorStore_cleanObsoleteCandidates(t *testing.T) {
	t.Parallel()

	var (
		validators = validators.NewECDSAValidatorSet(
			ecdsaValidator1,
		)

		initialCandidates = []*store.Candidate{
			{
				Validator: ecdsaValidator1,
				Authorize: true,
			},
			{
				Validator: ecdsaValidator1,
				Authorize: false,
			},
			{
				Validator: ecdsaValidator2,
				Authorize: true,
			},
			{
				Validator: ecdsaValidator2,
				Authorize: false,
			},
		}

		finalCandidates = []*store.Candidate{
			{
				Validator: ecdsaValidator1,
				Authorize: false,
			},
			{
				Validator: ecdsaValidator2,
				Authorize: true,
			},
		}
	)

	snapshotStore := newTestSnapshotValidatorStore(
		nil,
		nil,
		0,
		nil,
		initialCandidates,
		0,
	)

	snapshotStore.cleanObsoleteCandidates(validators)

	assert.Equal(
		t,
		finalCandidates,
		snapshotStore.candidates,
	)
}

func TestSnapshotValidatorStore_pickOneCandidate(t *testing.T) {
	t.Parallel()

	var (
		proposer = ecdsaValidator1.Addr()

		candidates = []*store.Candidate{
			{
				Validator: ecdsaValidator2,
				Authorize: true,
			},
			{
				Validator: ecdsaValidator3,
				Authorize: true,
			},
		}

		snapshot = &Snapshot{
			Votes: []*store.Vote{
				// validator1 has voted to validator2 already
				newTestVote(ecdsaValidator2, ecdsaValidator1.Address, true),
			},
		}

		expected = &store.Candidate{
			Validator: ecdsaValidator3,
			Authorize: true,
		}
	)

	snapshotStore := newTestSnapshotValidatorStore(
		nil,
		nil,
		0,
		nil,
		candidates,
		0,
	)

	candidate := snapshotStore.pickOneCandidate(snapshot, proposer)

	assert.Equal(
		t,
		expected,
		candidate,
	)
}

func TestSnapshotValidatorStore_saveSnapshotIfChanged(t *testing.T) {
	t.Parallel()

	var (
		headerHeight uint64 = 30
		headerHash          = types.BytesToHash(crypto.Keccak256([]byte{byte(headerHeight)}))
		header              = &types.Header{
			Number: headerHeight,
			Hash:   headerHash,
		}

		parentVals = validators.NewECDSAValidatorSet(
			ecdsaValidator1,
		)

		newVals = validators.NewECDSAValidatorSet(
			ecdsaValidator1,
			ecdsaValidator2,
		)

		parentVotes = []*store.Vote{}

		newVotes = []*store.Vote{
			newTestVote(ecdsaValidator2, ecdsaValidator1.Address, true),
		}
	)

	tests := []struct {
		name             string
		initialSnapshots []*Snapshot
		parentSnapshot   *Snapshot
		snapshot         *Snapshot
		finalSnapshots   []*Snapshot
	}{
		{
			name: "shouldn't add a new snapshot if the snapshot equals to the parent snapshot",
			initialSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
			},
			parentSnapshot: &Snapshot{Number: 20, Set: parentVals, Votes: parentVotes},
			snapshot:       &Snapshot{Number: headerHeight, Set: parentVals, Votes: parentVotes},
			finalSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
			},
		},
		{
			name: "should add a new snapshot if the snapshot equals to the parent snapshot",
			initialSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
			},
			parentSnapshot: &Snapshot{
				Number: 20,
				Set:    parentVals,
				Votes:  parentVotes,
			},
			snapshot: &Snapshot{
				Number: headerHeight,
				Hash:   header.Hash.String(),
				Set:    newVals,
				Votes:  newVotes,
			},
			finalSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
				{
					Number: headerHeight,
					Hash:   header.Hash.String(),
					Set:    newVals,
					Votes:  newVotes,
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				nil,
				20,
				test.initialSnapshots,
				nil,
				0,
			)

			snapshotStore.saveSnapshotIfChanged(test.parentSnapshot, test.snapshot, header)

			assert.Equal(
				t,
				test.finalSnapshots,
				snapshotStore.GetSnapshots(),
			)
		})
	}
}

func TestSnapshotValidatorStore_resetSnapshot(t *testing.T) {
	t.Parallel()

	var (
		headerHeight uint64 = 30
		headerHash          = types.BytesToHash(crypto.Keccak256([]byte{byte(headerHeight)}))
		header              = &types.Header{
			Number: headerHeight,
			Hash:   headerHash,
		}

		vals = validators.NewECDSAValidatorSet(
			ecdsaValidator1,
		)

		parentVotes = []*store.Vote{
			newTestVote(ecdsaValidator2, ecdsaValidator1.Address, true),
		}
	)

	tests := []struct {
		name             string
		initialSnapshots []*Snapshot
		parentSnapshot   *Snapshot
		snapshot         *Snapshot
		finalSnapshots   []*Snapshot
	}{
		{
			name: "should add a new snapshot without votes if the parent snapshot has votes",
			initialSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
			},
			parentSnapshot: &Snapshot{Number: 20, Set: vals, Votes: parentVotes},
			snapshot:       &Snapshot{Number: headerHeight, Set: vals, Votes: parentVotes},
			finalSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
				{
					Number: headerHeight,
					Hash:   headerHash.String(),
					Set:    vals,
					Votes:  nil,
				},
			},
		},
		{
			name: "shouldn't add if the parent snapshot doesn't have votes",
			initialSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
			},
			parentSnapshot: &Snapshot{Number: 20, Set: vals, Votes: []*store.Vote{}},
			snapshot:       &Snapshot{Number: headerHeight, Set: vals, Votes: parentVotes},
			finalSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				nil,
				20,
				test.initialSnapshots,
				nil,
				0,
			)

			snapshotStore.resetSnapshot(test.parentSnapshot, test.snapshot, header)

			assert.Equal(
				t,
				test.finalSnapshots,
				snapshotStore.GetSnapshots(),
			)
		})
	}
}

func TestSnapshotValidatorStore_removeLowerSnapshots(t *testing.T) {
	t.Parallel()

	var (
		epochSize uint64 = 10
	)

	tests := []struct {
		name             string
		initialSnapshots []*Snapshot
		height           uint64
		finalSnapshots   []*Snapshot
	}{
		{
			name: "should remove the old snapshots",
			initialSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
				{Number: 30},
				{Number: 40},
				{Number: 50},
			},
			height: 51, // the beginning of the current epoch is 50
			finalSnapshots: []*Snapshot{
				{Number: 30},
				{Number: 40},
				{Number: 50},
			},
		},
		{
			name: "shouldn't remove in case of epoch 0-2",
			initialSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
			},
			height: 20,
			finalSnapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshotStore := newTestSnapshotValidatorStore(
				nil,
				nil,
				20,
				test.initialSnapshots,
				nil,
				epochSize,
			)

			snapshotStore.removeLowerSnapshots(test.height)

			assert.Equal(
				t,
				test.finalSnapshots,
				snapshotStore.GetSnapshots(),
			)
		})
	}
}

func TestSnapshotValidatorStore_processVote(t *testing.T) {
	var (
		headerNumber uint64 = 21

		initialECDSAValidatorSet = validators.NewECDSAValidatorSet(
			ecdsaValidator1,
			ecdsaValidator2,
		)

		initialBLSValidatorSet = validators.NewBLSValidatorSet(
			blsValidator1,
			blsValidator2,
		)
	)

	tests := []struct {
		name             string
		header           *types.Header
		candidateType    validators.ValidatorType
		proposer         types.Address
		snapshot         *Snapshot
		expectedErr      error
		expectedSnapshot *Snapshot
	}{
		{
			name: "should return ErrIncorrectNonce if header.Nonce is invalid",
			header: &types.Header{
				Nonce: types.Nonce{0x01},
			},
			proposer:         types.ZeroAddress,
			snapshot:         nil,
			expectedErr:      ErrIncorrectNonce,
			expectedSnapshot: nil,
		},
		{
			name: "should return ErrInvalidValidatorType if the signer returns invalid type",
			header: &types.Header{
				Nonce: nonceAuthVote,
			},
			candidateType:    validators.ValidatorType("fake type"),
			proposer:         types.ZeroAddress,
			snapshot:         nil,
			expectedErr:      validators.ErrInvalidValidatorType,
			expectedSnapshot: nil,
		},
		{
			name: "should return error when failing parse Miner field as a validator",
			header: &types.Header{
				Nonce: nonceAuthVote,
				Miner: []byte{0x1, 0x1},
			},
			candidateType:    validators.BLSValidatorType,
			proposer:         types.ZeroAddress,
			snapshot:         nil,
			expectedErr:      errors.New("value is not of type array"),
			expectedSnapshot: nil,
		},
		{
			name: "should update latest block height if the ECDSA candidate for addition is in the validator set already",
			header: &types.Header{
				Number: headerNumber,
				Nonce:  nonceAuthVote,
				Miner:  ecdsaValidator2.Address.Bytes(),
			},
			candidateType: validators.ECDSAValidatorType,
			proposer:      types.ZeroAddress,
			snapshot: &Snapshot{
				Set: initialECDSAValidatorSet,
			},
			expectedErr: nil,
			expectedSnapshot: &Snapshot{
				Set: initialECDSAValidatorSet,
			},
		},
		{
			name: "should update latest block height if the BLS candidate for deletion isn't in the validator set",
			header: &types.Header{
				Number: headerNumber,
				Nonce:  nonceDropVote,
				Miner:  blsValidator3.Bytes(),
			},
			candidateType: validators.BLSValidatorType,
			proposer:      types.ZeroAddress,
			snapshot: &Snapshot{
				Set: initialBLSValidatorSet,
			},
			expectedErr: nil,
			expectedSnapshot: &Snapshot{
				Set: initialBLSValidatorSet,
			},
		},
		{
			name: "should return ErrMultipleVotesBySameValidator" +
				" if the snapshot has multiple votes for the same candidate by the same proposer",
			header: &types.Header{
				Number: headerNumber,
				Nonce:  nonceAuthVote,
				Miner:  ecdsaValidator3.Bytes(),
			},
			candidateType: validators.ECDSAValidatorType,
			proposer:      ecdsaValidator1.Address,
			snapshot: &Snapshot{
				Set: initialECDSAValidatorSet,
				Votes: []*store.Vote{
					// duplicated votes
					newTestVote(ecdsaValidator3, ecdsaValidator1.Address, true),
					newTestVote(ecdsaValidator3, ecdsaValidator1.Address, true),
				},
			},
			expectedErr: ErrMultipleVotesBySameValidator,
			expectedSnapshot: &Snapshot{
				Set: initialECDSAValidatorSet,
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator3, ecdsaValidator1.Address, true),
					newTestVote(ecdsaValidator3, ecdsaValidator1.Address, true),
				},
			},
		},
		{
			name: "should add a vote to the snapshot and save in the snapshots",
			header: &types.Header{
				Number: headerNumber,
				Nonce:  nonceAuthVote,
				Miner:  ecdsaValidator3.Bytes(),
			},
			candidateType: validators.ECDSAValidatorType,
			proposer:      ecdsaValidator1.Address,
			snapshot: &Snapshot{
				Set:   initialECDSAValidatorSet,
				Votes: []*store.Vote{},
			},
			expectedErr: nil,
			expectedSnapshot: &Snapshot{
				Set: initialECDSAValidatorSet,
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator3, ecdsaValidator1.Address, true),
				},
			},
		},
		{
			name: "should drop a BLS validator from validator set and remove the all votes by the deleted validator",
			header: &types.Header{
				Number: headerNumber,
				Nonce:  nonceDropVote,
				Miner:  blsValidator2.Bytes(),
			},
			candidateType: validators.BLSValidatorType,
			proposer:      blsValidator1.Address,
			snapshot: &Snapshot{
				Set: initialBLSValidatorSet,
				Votes: []*store.Vote{
					// Votes by Validator 1
					{
						Candidate: blsValidator3,
						Validator: blsValidator1.Address,
						Authorize: true,
					},
					// Votes by Validator 2
					{
						Candidate: blsValidator2,
						Validator: blsValidator2.Address,
						Authorize: false,
					},
					{
						Candidate: blsValidator3,
						Validator: blsValidator2.Address,
						Authorize: true,
					},
				},
			},
			expectedErr: nil,
			expectedSnapshot: &Snapshot{
				Set: validators.NewBLSValidatorSet(
					blsValidator1,
				),
				Votes: []*store.Vote{
					// keep only the votes by validator 1
					{
						Candidate: blsValidator3,
						Validator: blsValidator1.Address,
						Authorize: true,
					},
				},
			},
		},
		{
			name: "should add a ECDSA candidate to validator set and clear votes for the candidate",
			header: &types.Header{
				Number: headerNumber,
				Nonce:  nonceAuthVote,
				Miner:  ecdsaValidator3.Bytes(),
			},
			candidateType: validators.ECDSAValidatorType,
			proposer:      ecdsaValidator1.Address,
			snapshot: &Snapshot{
				Set: initialECDSAValidatorSet,
				Votes: []*store.Vote{
					// Validator2 has voted already
					{
						Candidate: ecdsaValidator3,
						Validator: ecdsaValidator2.Address,
						Authorize: true,
					},
				},
			},
			expectedErr: nil,
			expectedSnapshot: &Snapshot{
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
					ecdsaValidator2,
					// add the new validator
					ecdsaValidator3,
				),
				// clear votes
				Votes: []*store.Vote{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testHelper.AssertErrorMessageContains(
				t,
				test.expectedErr,
				processVote(
					test.snapshot,
					test.header,
					test.candidateType,
					test.proposer,
				),
			)

			assert.Equal(
				t,
				test.expectedSnapshot,
				test.snapshot,
			)
		})
	}
}

func Test_validatorToMiner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		validator   validators.Validator
		expectedRes []byte
		expectedErr error
	}{
		{
			name:        "ECDSAValidator",
			validator:   ecdsaValidator1,
			expectedRes: ecdsaValidator1.Address.Bytes(),
			expectedErr: nil,
		},
		{
			name:        "BLSValidator",
			validator:   blsValidator1,
			expectedRes: blsValidator1.Bytes(),
			expectedErr: nil,
		},
		{
			name:        "fake validator",
			validator:   &fakeValidator{},
			expectedRes: nil,
			expectedErr: validators.ErrInvalidValidatorType,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := validatorToMiner(test.validator)

			assert.Equal(
				t,
				test.expectedRes,
				res,
			)

			assert.Equal(
				t,
				test.expectedErr,
				err,
			)
		})
	}
}

func Test_minerToValidator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		validatorType validators.ValidatorType
		miner         []byte
		expectedRes   validators.Validator
		expectedErr   error
	}{
		{
			name:          "ECDSAValidator",
			validatorType: validators.ECDSAValidatorType,
			miner:         ecdsaValidator1.Address.Bytes(),
			expectedRes:   ecdsaValidator1,
			expectedErr:   nil,
		},
		{
			name:          "BLSValidator",
			validatorType: validators.BLSValidatorType,
			miner:         blsValidator1.Bytes(),
			expectedRes:   blsValidator1,
			expectedErr:   nil,
		},
		{
			name:          "fake validator",
			validatorType: validators.ValidatorType("fake"),
			miner:         ecdsaValidator1.Address.Bytes(),
			expectedRes:   nil,
			expectedErr:   validators.ErrInvalidValidatorType,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := minerToValidator(test.validatorType, test.miner)

			assert.Equal(
				t,
				test.expectedRes,
				res,
			)

			assert.Equal(
				t,
				test.expectedErr,
				err,
			)
		})
	}
}
