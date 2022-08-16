package snapshot

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/valset"
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
			"%s"
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
			"%s"
		]
	}`,
		hash,
		number,
		validators.BLSValidatorType,
		voteAuthorize,
		voteCandidate.Address,
		voteCandidate.BLSPublicKey,
		voteValidator,
		setValidator.String(),
	)
}

func newTestVote(
	candidate validators.Validator,
	validator types.Address,
	authorize bool,
) *valset.Vote {
	return &valset.Vote{
		Validator: validator,
		Candidate: candidate,
		Authorize: authorize,
	}
}

func TestSnapshotMarshalJSON(t *testing.T) {
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
		vote := newTestVote(ecdsaValidator2, addr1, true)

		testMarshalJSON(
			t,
			&Snapshot{
				Number: testNumber,
				Hash:   testHash.String(),
				Votes: []*valset.Vote{
					vote,
				},
				Set: &validators.ECDSAValidators{
					ecdsaValidator1,
				},
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
		vote := newTestVote(blsValidator2, addr1, false)

		testMarshalJSON(
			t,
			&Snapshot{
				Number: testNumber,
				Hash:   testHash.String(),
				Votes: []*valset.Vote{
					vote,
				},
				Set: &validators.BLSValidators{
					blsValidator1,
				},
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
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator2.Addr(), true),
				},
				Set: &validators.ECDSAValidators{
					ecdsaValidator2,
				},
			},
		)
	})

	t.Run("BLSValidators", func(t *testing.T) {
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
				Votes: []*valset.Vote{
					newTestVote(blsValidator1, blsValidator2.Addr(), false),
				},
				Set: &validators.BLSValidators{
					blsValidator2,
				},
			},
		)
	})
}

func TestSnapshotEqual(t *testing.T) {
	tests := []struct {
		name     string
		s1       *Snapshot
		s2       *Snapshot
		expected bool
	}{
		{
			name: "should return true if they're equal",
			s1: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true),
				},
				Set: &validators.ECDSAValidators{
					ecdsaValidator1,
				},
			},
			s2: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true),
				},
				Set: &validators.ECDSAValidators{
					ecdsaValidator1,
				},
			},
			expected: true,
		},
		{
			name: "should return false if the sizes of Votes doesn't match with each other",
			s1: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true),
				},
				Set: &validators.ECDSAValidators{
					ecdsaValidator1,
				},
			},
			s2: &Snapshot{
				Votes: []*valset.Vote{},
				Set: &validators.ECDSAValidators{
					ecdsaValidator1,
				},
			},
			expected: false,
		},
		{
			name: "should return false if Votes don't match with each other",
			s1: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true),
				},
				Set: &validators.ECDSAValidators{
					ecdsaValidator1,
				},
			},
			s2: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator3, ecdsaValidator1.Addr(), true),
				},
				Set: &validators.ECDSAValidators{
					ecdsaValidator1,
				},
			},
			expected: false,
		},
		{
			name: "should return true if Sets doesn't match with each other",
			s1: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator3, ecdsaValidator1.Addr(), true),
				},
				Set: &validators.ECDSAValidators{
					ecdsaValidator1,
				},
			},
			s2: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator3, ecdsaValidator1.Addr(), true),
				},
				Set: &validators.ECDSAValidators{
					ecdsaValidator2,
				},
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				test.s1.Equal(test.s2),
			)
		})
	}
}

func TestSnapshotCount(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *Snapshot
		fn       func(v *valset.Vote) bool
		expected int
		visited  []*valset.Vote
	}{
		{
			name: "should return true if they're equal",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(ecdsaValidator2, ecdsaValidator2.Addr(), false),
					newTestVote(ecdsaValidator3, ecdsaValidator3.Addr(), true),
				},
			},
			fn: func(v *valset.Vote) bool {
				// count all
				return true
			},
			expected: 3,
			visited: []*valset.Vote{
				newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
				newTestVote(ecdsaValidator2, ecdsaValidator2.Addr(), false),
				newTestVote(ecdsaValidator3, ecdsaValidator3.Addr(), true),
			},
		},
		{
			name: "shouldn't count but visit all",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(blsValidator2, ecdsaValidator2.Addr(), false),
				},
			},
			fn: func(v *valset.Vote) bool {
				// don't count
				return false
			},
			expected: 0,
			visited: []*valset.Vote{
				newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
				newTestVote(blsValidator2, ecdsaValidator2.Addr(), false),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			visited := make([]*valset.Vote, 0, len(test.snapshot.Votes))

			res := test.snapshot.Count(func(v *valset.Vote) bool {
				visited = append(visited, v)

				return test.fn(v)
			})

			assert.Equal(t, test.expected, res)
			assert.Equal(t, test.visited, visited)
		})
	}
}

func TestSnapshotAddVote(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *Snapshot
		vote     *valset.Vote
		expected []*valset.Vote
	}{
		{
			name: "should add ECDSA Validator Vote",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
				},
			},
			vote: newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), false),
			expected: []*valset.Vote{
				newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
				newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), false),
			},
		},
		{
			name: "should add BLS Validator Vote",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
				},
			},
			vote: newTestVote(blsValidator2, ecdsaValidator2.Addr(), false),
			expected: []*valset.Vote{
				newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
				newTestVote(blsValidator2, ecdsaValidator2.Addr(), false),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
	tests := []struct {
		name     string
		snapshot *Snapshot
	}{
		{
			name: "should copy ECDSA Snapshot",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
				},
				Set: &validators.ECDSAValidators{
					ecdsaValidator1,
					ecdsaValidator2,
				},
			},
		},
		{
			name: "should copy BLS Snapshot",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
				},
				Set: &validators.BLSValidators{
					blsValidator1,
					blsValidator2,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
				Votes: []*valset.Vote{
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
		t.Run(test.name, func(t *testing.T) {
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
	tests := []struct {
		name      string
		snapshot  *Snapshot
		candidate validators.Validator
		expected  int
	}{
		{
			name: "should return count of the votes whose Candidate equal to the given field",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
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
		t.Run(test.name, func(t *testing.T) {
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
	tests := []struct {
		name     string
		snapshot *Snapshot
		fn       func(v *valset.Vote) bool
		expected []*valset.Vote
	}{
		{
			name: "should remove all Votes from Votes",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), true),
					newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), true),
				},
			},
			fn: func(v *valset.Vote) bool {
				// remove all
				return true
			},
			expected: []*valset.Vote{},
		},
		{
			name: "should removes only Votes created by Validator 1",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(blsValidator2, ecdsaValidator2.Addr(), true),
				},
			},
			fn: func(v *valset.Vote) bool {
				return v.Validator == ecdsaValidator1.Address
			},
			expected: []*valset.Vote{
				newTestVote(blsValidator2, ecdsaValidator2.Addr(), true),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
	tests := []struct {
		name     string
		snapshot *Snapshot
		voter    types.Address
		expected []*valset.Vote
	}{
		{
			name: "should remove all Votes",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(blsValidator2, ecdsaValidator1.Addr(), false),
				},
			},
			voter:    ecdsaValidator1.Address,
			expected: []*valset.Vote{},
		},
		{
			name: "should removes only Votes created by Validator 1",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), false),
					newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), false),
				},
			},
			voter: ecdsaValidator1.Address,
			expected: []*valset.Vote{
				newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), false),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
	tests := []struct {
		name      string
		snapshot  *Snapshot
		candidate validators.Validator
		expected  []*valset.Vote
	}{
		{
			name: "should remove all Votes",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(blsValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(blsValidator1, ecdsaValidator2.Addr(), false),
				},
			},
			candidate: blsValidator1,
			expected:  []*valset.Vote{},
		},
		{
			name: "should removes only Votes for Validator 1",
			snapshot: &Snapshot{
				Votes: []*valset.Vote{
					newTestVote(ecdsaValidator1, ecdsaValidator1.Addr(), true),
					newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), false),
					newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), false),
				},
			},
			candidate: ecdsaValidator1,
			expected: []*valset.Vote{
				newTestVote(ecdsaValidator2, ecdsaValidator1.Addr(), false),
				newTestVote(ecdsaValidator3, ecdsaValidator2.Addr(), false),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.snapshot.RemoveVotesByCandidate(
				test.candidate,
			)

			assert.Equal(t, test.expected, test.snapshot.Votes)
			// make sure the size and capacity equal with each other
			assert.Equal(t, len(test.snapshot.Votes), cap(test.snapshot.Votes))
		})
	}
}

func Test_snapshotStore_newSnapshotStore(t *testing.T) {

}

func Test_snapshotSortedListLen(t *testing.T) {
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
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				test.list.Len(),
			)
		})
	}
}

func Test_snapshotSortedListSwap(t *testing.T) {
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
		t.Run(test.name, func(t *testing.T) {
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
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				test.list.Less(0, 1),
			)
		})
	}
}
