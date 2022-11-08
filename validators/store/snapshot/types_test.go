package snapshot

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/stretchr/testify/assert"
)

var (
	testNumber uint64 = 10
	testHash          = types.BytesToHash(crypto.Keccak256([]byte{byte(testNumber)}))
)

func createExampleECDSASnapshotJSON(
	hash types.Hash,
	number uint64,
	voteAuthorize bool,
	voteCandidate *validators.ECDSAValidator,
	voteValidator types.Address,
	setValidator *validators.ECDSAValidator,
) string {
	return fmt.Sprintf(`{
		"Hash": "%s",
		"Number": %d,
		"Type": "%s",
		"Votes": [
			{
				"Authorize": %t,
				"Candidate": {
					"Address": "%s"
				},
				"Validator": "%s"
			}
		],
		"Set": [
			{
				"Address": "%s"
			}
		]
	}`,
		hash,
		number,
		validators.ECDSAValidatorType,
		voteAuthorize,
		voteCandidate.Addr(),
		voteValidator,
		setValidator.String(),
	)
}

func createExampleBLSSnapshotJSON(
	hash types.Hash,
	number uint64,
	voteAuthorize bool,
	voteCandidate *validators.BLSValidator,
	voteValidator types.Address,
	setValidator *validators.BLSValidator,
) string {
	return fmt.Sprintf(`{
		"Hash": "%s",
		"Number": %d,
		"Type": "%s",
		"Votes": [
			{
				"Authorize": %t,
				"Candidate": {
					"Address": "%s",
					"BLSPublicKey": "%s"
				},
				"Validator": "%s"
			}
		],
		"Set": [
			{
				"Address": "%s",
				"BLSPublicKey": "%s"
			}
		]
	}`,
		hash,
		number,
		validators.BLSValidatorType,
		voteAuthorize,
		voteCandidate.Address,
		voteCandidate.BLSPublicKey,
		voteValidator,
		setValidator.Address,
		setValidator.BLSPublicKey,
	)
}

func newTestVote(
	candidate validators.Validator,
	validator types.Address,
	authorize bool,
) *store.Vote {
	return &store.Vote{
		Validator: validator,
		Candidate: candidate,
		Authorize: authorize,
	}
}

func TestSnapshotMarshalJSON(t *testing.T) {
	t.Parallel()

	testMarshalJSON := func(
		t *testing.T,
		data interface{},
		expectedJSON string, // can be beautified
	) {
		t.Helper()

		res, err := json.Marshal(data)

		assert.NoError(t, err)
		assert.JSONEq(
			t,
			strings.TrimSpace(expectedJSON),
			string(res),
		)
	}

	t.Run("ECDSAValidators", func(t *testing.T) {
		t.Parallel()

		vote := newTestVote(ecdsaValidator2, addr1, true)

		testMarshalJSON(
			t,
			&Snapshot{
				Number: testNumber,
				Hash:   testHash.String(),
				Votes: []*store.Vote{
					vote,
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
			},
			createExampleECDSASnapshotJSON(
				testHash,
				testNumber,
				vote.Authorize,
				ecdsaValidator2,
				vote.Validator,
				ecdsaValidator1,
			),
		)
	})

	t.Run("BLSValidators", func(t *testing.T) {
		t.Parallel()

		vote := newTestVote(blsValidator2, addr1, false)

		testMarshalJSON(
			t,
			&Snapshot{
				Number: testNumber,
				Hash:   testHash.String(),
				Votes: []*store.Vote{
					vote,
				},
				Set: validators.NewBLSValidatorSet(
					blsValidator1,
				),
			},
			createExampleBLSSnapshotJSON(
				testHash,
				testNumber,
				vote.Authorize,
				blsValidator2,
				blsValidator1.Addr(),
				blsValidator1,
			),
		)
	})
}

func TestSnapshotUnmarshalJSON(t *testing.T) {
	t.Parallel()

	testUnmarshalJSON := func(
		t *testing.T,
		jsonStr string,
		target interface{},
		expected interface{},
	) {
		t.Helper()

		err := json.Unmarshal([]byte(jsonStr), target)

		assert.NoError(t, err)
		assert.Equal(t, expected, target)
	}

	t.Run("ECDSAValidators", func(t *testing.T) {
		t.Parallel()

		testUnmarshalJSON(
			t,
			createExampleECDSASnapshotJSON(
				testHash,
				testNumber,
				true,
				ecdsaValidator1,
				ecdsaValidator2.Addr(),
				ecdsaValidator2,
			),
			&Snapshot{},
			&Snapshot{
				Number: testNumber,
				Hash:   testHash.String(),
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator2.Addr(), true),
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator2,
				),
			},
		)
	})

	t.Run("ECDSAValidators (Legacy format)", func(t *testing.T) {
		t.Parallel()

		testUnmarshalJSON(
			t,
			fmt.Sprintf(`
			{
				"Number": %d,
				"Hash": "%s",
				"Votes": [
					{
						"Validator": "%s",
						"Address": "%s",
						"Authorize": %t
					}
				],
				"Set": [
					"%s"
				]
			}
			`,
				testNumber,
				testHash,
				ecdsaValidator2.Addr(),
				ecdsaValidator1.Addr(),
				true,
				ecdsaValidator2.Addr(),
			),
			&Snapshot{},
			&Snapshot{
				Number: testNumber,
				Hash:   testHash.String(),
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator2.Addr(), true),
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator2,
				),
			},
		)
	})

	t.Run("BLSValidators", func(t *testing.T) {
		t.Parallel()

		testUnmarshalJSON(
			t,
			createExampleBLSSnapshotJSON(
				testHash,
				testNumber,
				false,
				blsValidator1,
				ecdsaValidator2.Addr(),
				blsValidator2,
			),
			&Snapshot{},
			&Snapshot{
				Number: testNumber,
				Hash:   testHash.String(),
				Votes: []*store.Vote{
					newTestVote(blsValidator1, blsValidator2.Addr(), false),
				},
				Set: validators.NewBLSValidatorSet(
					blsValidator2,
				),
			},
		)
	})

	t.Run("error handling", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			jsonStr string
		}{
			{
				name:    "should return error if UnmarshalJSON for raw failed",
				jsonStr: "[]",
			},
			{
				name: "should error if parsing Type is failed",
				jsonStr: `{
					"Number": 0,
					"Hash": "0x1",
					"Type": "fake",
					"Votes": [],
					"Set": []
				}`,
			},
			{
				name: "should error if unmarshal Votes is failed",
				jsonStr: `{
					"Number": 0,
					"Hash": "0x1",
					"Type": "ecdsa",
					"Votes": [
						1
					],
					"Set": []
				}`,
			},
			{
				name: "should return error if unmarshal Set is failed",
				jsonStr: `{
					"Number": 0,
					"Hash": "0x1",
					"Type": "ecdsa",
					"Votes": [],
					"Set": [
						1
					]
				}`,
			},
		}

		for _, test := range tests {
			test := test

			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				assert.Error(
					t,
					json.Unmarshal([]byte(test.jsonStr), &Snapshot{}),
				)
			})
		}
	})
}

func TestSnapshotEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		s1       *Snapshot
		s2       *Snapshot
		expected bool
	}{
		{
			name: "should return true if they're equal",
			s1: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true),
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
			},
			s2: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true),
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
			},
			expected: true,
		},
		{
			name: "should return false if the sizes of Votes doesn't match with each other",
			s1: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true),
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
			},
			s2: &Snapshot{
				Votes: []*store.Vote{},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
			},
			expected: false,
		},
		{
			name: "should return false if Votes don't match with each other",
			s1: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true),
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
			},
			s2: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator3, ecdsaValidator1.Addr(), true),
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
			},
			expected: false,
		},
		{
			name: "should return true if Sets doesn't match with each other",
			s1: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator3, ecdsaValidator1.Addr(), true),
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
			},
			s2: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator3, ecdsaValidator1.Addr(), true),
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator2,
				),
			},
			expected: false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.s1.Equal(test.s2),
			)
		})
	}
}

func TestSnapshotCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snapshot *Snapshot
		fn       func(v *store.Vote) bool
		expected int
		visited  []*store.Vote
	}{
		{
			name: "should return true if they're equal",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(ecdsaValidator2, ecdsaValidator2.Addr(), false),
					newTestVote(ecdsaValidator3, ecdsaValidator3.Addr(), true),
				},
			},
			fn: func(v *store.Vote) bool {
				// count all
				return true
			},
			expected: 3,
			visited: []*store.Vote{
				newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
				newTestVote(ecdsaValidator2, ecdsaValidator2.Addr(), false),
				newTestVote(ecdsaValidator3, ecdsaValidator3.Addr(), true),
			},
		},
		{
			name: "shouldn't count but visit all",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(blsValidator2, ecdsaValidator2.Addr(), false),
				},
			},
			fn: func(v *store.Vote) bool {
				// don't count
				return false
			},
			expected: 0,
			visited: []*store.Vote{
				newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
				newTestVote(blsValidator2, ecdsaValidator2.Addr(), false),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			visited := make([]*store.Vote, 0, len(test.snapshot.Votes))

			res := test.snapshot.Count(func(v *store.Vote) bool {
				visited = append(visited, v)

				return test.fn(v)
			})

			assert.Equal(t, test.expected, res)
			assert.Equal(t, test.visited, visited)
		})
	}
}

func TestSnapshotAddVote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snapshot *Snapshot
		vote     *store.Vote
		expected []*store.Vote
	}{
		{
			name: "should add ECDSA Validator Vote",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
				},
			},
			vote: newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), false),
			expected: []*store.Vote{
				newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
				newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), false),
			},
		},
		{
			name: "should add BLS Validator Vote",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
				},
			},
			vote: newTestVote(blsValidator2, ecdsaValidator2.Addr(), false),
			expected: []*store.Vote{
				newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
				newTestVote(blsValidator2, ecdsaValidator2.Addr(), false),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.snapshot.AddVote(
				test.vote.Validator,
				test.vote.Candidate,
				test.vote.Authorize,
			)

			assert.Equal(t, test.expected, test.snapshot.Votes)
		})
	}
}

func TestSnapshotCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snapshot *Snapshot
	}{
		{
			name: "should copy ECDSA Snapshot",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
				},
				Set: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
					ecdsaValidator2,
				),
			},
		},
		{
			name: "should copy BLS Snapshot",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
				},
				Set: validators.NewBLSValidatorSet(
					blsValidator1,
					blsValidator2,
				),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			copied := test.snapshot.Copy()

			// check fields
			assert.Equal(t, test.snapshot, copied)

			// check addresses of Set are different
			assert.NotSame(t, test.snapshot.Set, copied.Set)

			// check addresses of Votes are different
			assert.Equal(t, len(test.snapshot.Votes), len(copied.Votes))
			for idx := range test.snapshot.Votes {
				assert.NotSame(t, test.snapshot.Votes[idx], copied.Votes[idx])
			}
		})
	}
}

func TestSnapshotCountByVoterAndCandidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		snapshot  *Snapshot
		voter     types.Address
		candidate validators.Validator
		expected  int
	}{
		{
			name: "should return count of the votes whose Validator and Candidate equal to the given fields",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true), // not match
					newTestVote(ecdsaValidator2, ecdsaValidator2.Addr(), true), // not match
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true), // match
				},
			},
			voter:     ecdsaValidator1.Addr(),
			candidate: ecdsaValidator2,
			expected:  1,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.snapshot.CountByVoterAndCandidate(
					test.voter,
					test.candidate,
				),
			)
		})
	}
}

func TestSnapshotCountByCandidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		snapshot  *Snapshot
		candidate validators.Validator
		expected  int
	}{
		{
			name: "should return count of the votes whose Candidate equal to the given field",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true), // match
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true), // not match
					newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), true), // not match
				},
			},
			candidate: ecdsaValidator1,
			expected:  1,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.snapshot.CountByCandidate(
					test.candidate,
				),
			)
		})
	}
}

func TestSnapshotRemoveVotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snapshot *Snapshot
		fn       func(v *store.Vote) bool
		expected []*store.Vote
	}{
		{
			name: "should remove all Votes from Votes",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true),
					newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), true),
				},
			},
			fn: func(v *store.Vote) bool {
				// remove all
				return true
			},
			expected: []*store.Vote{},
		},
		{
			name: "should removes only Votes created by Validator 1",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(blsValidator2, ecdsaValidator2.Addr(), true),
				},
			},
			fn: func(v *store.Vote) bool {
				return v.Validator == ecdsaValidator1.Address
			},
			expected: []*store.Vote{
				newTestVote(blsValidator2, ecdsaValidator2.Addr(), true),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.snapshot.RemoveVotes(
				test.fn,
			)

			assert.Equal(t, test.expected, test.snapshot.Votes)
			// make sure the size and capacity equal with each other
			assert.Equal(t, len(test.snapshot.Votes), cap(test.snapshot.Votes))
		})
	}
}

func TestSnapshotRemoveVotesByVoter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snapshot *Snapshot
		voter    types.Address
		expected []*store.Vote
	}{
		{
			name: "should remove all Votes",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(blsValidator2, ecdsaValidator1.Addr(), false),
				},
			},
			voter:    ecdsaValidator1.Address,
			expected: []*store.Vote{},
		},
		{
			name: "should removes only Votes created by Validator 1",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), false),
					newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), false),
				},
			},
			voter: ecdsaValidator1.Address,
			expected: []*store.Vote{
				newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), false),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.snapshot.RemoveVotesByVoter(
				test.voter,
			)

			assert.Equal(t, test.expected, test.snapshot.Votes)
			// make sure the size and capacity equal with each other
			assert.Equal(t, len(test.snapshot.Votes), cap(test.snapshot.Votes))
		})
	}
}

func TestSnapshotRemoveVotesByCandidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		snapshot  *Snapshot
		candidate validators.Validator
		expected  []*store.Vote
	}{
		{
			name: "should remove all Votes",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(blsValidator1, ecdsaValidator2.Addr(), false),
				},
			},
			candidate: blsValidator1,
			expected:  []*store.Vote{},
		},
		{
			name: "should removes only Votes for Validator 1",
			snapshot: &Snapshot{
				Votes: []*store.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), false),
					newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), false),
				},
			},
			candidate: ecdsaValidator1,
			expected: []*store.Vote{
				newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), false),
				newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), false),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.snapshot.RemoveVotesByCandidate(
				test.candidate,
			)

			assert.Equal(t, test.expected, test.snapshot.Votes)
			// make sure the size and capacity equal with each other
			assert.Equal(t, len(test.snapshot.Votes), cap(test.snapshot.Votes))
		})
	}
}

func Test_snapshotSortedListLen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		list     *snapshotSortedList
		expected int
	}{
		{
			name: "should return the size",
			list: &snapshotSortedList{
				&Snapshot{},
				&Snapshot{},
			},
			expected: 2,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.list.Len(),
			)
		})
	}
}

func Test_snapshotSortedListSwap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		list     *snapshotSortedList
		i, j     int
		expected *snapshotSortedList
	}{
		{
			name: "should swap elements",
			list: &snapshotSortedList{
				&Snapshot{Number: 3},
				&Snapshot{Number: 2},
				&Snapshot{Number: 1},
			},
			i: 0,
			j: 2,
			expected: &snapshotSortedList{
				&Snapshot{Number: 1},
				&Snapshot{Number: 2},
				&Snapshot{Number: 3},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.list.Swap(test.i, test.j)

			assert.Equal(
				t,
				test.expected,
				test.list,
			)
		})
	}
}

func Test_snapshotSortedListLess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		list     *snapshotSortedList
		i, j     int
		expected bool
	}{
		{
			name: "should return true when list[i].Number < list[j].Number",
			list: &snapshotSortedList{
				&Snapshot{Number: 1},
				&Snapshot{Number: 3},
			},
			expected: true,
		},
		{
			name: "should return false when list[i].Number == list[j].Number",
			list: &snapshotSortedList{
				&Snapshot{Number: 2},
				&Snapshot{Number: 2},
			},
			expected: false,
		},
		{
			name: "should return false when list[i].Number > list[j].Number",
			list: &snapshotSortedList{
				&Snapshot{Number: 2},
				&Snapshot{Number: 1},
			},
			expected: false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.list.Less(0, 1),
			)
		})
	}
}

func Test_newSnapshotStore(t *testing.T) {
	t.Parallel()

	var (
		metadata = &SnapshotMetadata{
			LastBlock: 10,
		}

		snapshots = []*Snapshot{
			{Number: 1},
			{Number: 3},
		}
	)

	assert.Equal(
		t,
		&snapshotStore{
			lastNumber: metadata.LastBlock,
			list: snapshotSortedList(
				snapshots,
			),
		},
		newSnapshotStore(
			metadata,
			snapshots,
		),
	)
}

func Test_snapshotStore_getLastBlock(t *testing.T) {
	t.Parallel()

	var (
		metadata = &SnapshotMetadata{
			LastBlock: 10,
		}
	)

	store := newSnapshotStore(
		metadata,
		nil,
	)

	assert.Equal(
		t,
		metadata.LastBlock,
		store.getLastBlock(),
	)
}

func Test_snapshotStore_updateLastBlock(t *testing.T) {
	t.Parallel()

	var (
		metadata = &SnapshotMetadata{
			LastBlock: 10,
		}

		newLastBlock = uint64(20)
	)

	store := newSnapshotStore(
		metadata,
		nil,
	)

	store.updateLastBlock(newLastBlock)

	assert.Equal(
		t,
		newLastBlock,
		store.getLastBlock(),
	)
}

func Test_snapshotStore_deleteLower(t *testing.T) {
	t.Parallel()

	metadata := &SnapshotMetadata{
		LastBlock: 10,
	}

	testTable := []struct {
		name              string
		snapshots         []*Snapshot
		boundary          uint64
		expectedSnapshots []*Snapshot
	}{
		{
			"Drop lower-number snapshots",
			[]*Snapshot{
				{Number: 10},
				{Number: 19},
				{Number: 25},
				{Number: 30},
			},
			uint64(20),
			[]*Snapshot{
				{Number: 25},
				{Number: 30},
			},
		},
		{
			"Higher block value",
			[]*Snapshot{
				{Number: 10},
				{Number: 11},
				{Number: 12},
				{Number: 13},
				{Number: 14},
			},
			uint64(15),
			[]*Snapshot{
				{Number: 14},
			},
		},
		{
			// Single snapshots shouldn't be dropped
			"Single snapshot",
			[]*Snapshot{
				{Number: 10},
			},
			uint64(15),
			[]*Snapshot{
				{Number: 10},
			},
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			store := newSnapshotStore(
				metadata,
				testCase.snapshots,
			)

			store.deleteLower(testCase.boundary)

			assert.Equal(
				t,
				&snapshotStore{
					lastNumber: metadata.LastBlock,
					list:       testCase.expectedSnapshots,
				},
				store,
			)
		})
	}
}

func Test_snapshotStore_find(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		snapshots []*Snapshot
		input     uint64
		expected  *Snapshot
	}{
		{
			name:      "should return nil if the list is empty",
			snapshots: nil,
			input:     1,
			expected:  nil,
		},
		{
			name: "should return the last element if it's lower than given number",
			snapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
				{Number: 30},
			},
			input: 40,
			expected: &Snapshot{
				Number: 30,
			},
		},
		{
			name: "should return the first element if the given value is less than any snapshot",
			snapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
				{Number: 30},
			},
			input: 5,
			expected: &Snapshot{
				Number: 10,
			},
		},
		{
			name: "should return the element whose Number matches with the given number",
			snapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
				{Number: 30},
			},
			input: 20,
			expected: &Snapshot{
				Number: 20,
			},
		},
		{
			name: "should return the one before the element whose Number is bigger than the given value",
			snapshots: []*Snapshot{
				{Number: 10},
				{Number: 20},
				{Number: 30},
			},
			input: 29,
			expected: &Snapshot{
				Number: 20,
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := newSnapshotStore(
				&SnapshotMetadata{},
				test.snapshots,
			)

			assert.Equal(
				t,
				test.expected,
				store.find(test.input),
			)
		})
	}
}

func Test_snapshotStore_add(t *testing.T) {
	t.Parallel()

	var (
		snapshots = []*Snapshot{
			{Number: 30},
			{Number: 25},
			{Number: 20},
			{Number: 15},
			{Number: 10},
		}

		newSnapshot = &Snapshot{Number: 12}

		expected = []*Snapshot{
			// should be sorted in asc
			{Number: 10},
			{Number: 12},
			{Number: 15},
			{Number: 20},
			{Number: 25},
			{Number: 30},
		}
	)

	store := newSnapshotStore(
		&SnapshotMetadata{},
		snapshots,
	)

	store.add(newSnapshot)

	assert.Equal(
		t,
		&snapshotStore{
			list: snapshotSortedList(
				expected,
			),
		},
		store,
	)
}

func Test_snapshotStore_putByNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		initialSnapshots []*Snapshot
		newSnapshot      *Snapshot
		finalSnapshots   []*Snapshot
	}{
		{
			name: "should replace if the same Number snapshot exists in the list",
			initialSnapshots: []*Snapshot{
				{Number: 10, Hash: "10"},
				{Number: 20, Hash: "20"},
				{Number: 30, Hash: "30"},
			},
			newSnapshot: &Snapshot{
				Number: 20,
				Hash:   "xxx",
			},
			finalSnapshots: []*Snapshot{
				{Number: 10, Hash: "10"},
				{Number: 20, Hash: "xxx"},
				{Number: 30, Hash: "30"},
			},
		},
		{
			name: "should add if the same Number snapshot doesn't exist in the list",
			initialSnapshots: []*Snapshot{
				{Number: 10, Hash: "10"},
				{Number: 20, Hash: "20"},
				{Number: 30, Hash: "30"},
			},
			newSnapshot: &Snapshot{
				Number: 25,
				Hash:   "25",
			},
			finalSnapshots: []*Snapshot{
				{Number: 10, Hash: "10"},
				{Number: 20, Hash: "20"},
				{Number: 25, Hash: "25"},
				{Number: 30, Hash: "30"},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := newSnapshotStore(
				&SnapshotMetadata{},
				test.initialSnapshots,
			)

			store.putByNumber(test.newSnapshot)

			assert.Equal(
				t,
				test.finalSnapshots,
				[]*Snapshot(store.list),
			)
		})
	}
}
