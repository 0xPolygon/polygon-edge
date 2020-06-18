package backend

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/consensus/ibft/validator"
	"github.com/0xPolygon/minimal/types"
)

const (
	dbKeySnapshotPrefix = "istanbul-snapshot"
)

// Vote represents a single vote that an authorized validator made to modify the
// list of authorizations.
type Vote struct {
	Validator types.Address `json:"validator"` // Authorized validator that cast this vote
	Block     uint64        `json:"block"`     // Block number the vote was cast in (expire old votes)
	Address   types.Address `json:"address"`   // Account being voted on to change its authorization
	Authorize bool          `json:"authorize"` // Whether to authorize or deauthorize the voted account
}

// Tally is a simple vote tally to keep the current score of votes. Votes that
// go against the proposal aren't counted since it's equivalent to not voting.
type Tally struct {
	Authorize bool `json:"authorize"` // Whether the vote it about authorizing or kicking someone
	Votes     int  `json:"votes"`     // Number of votes until now wanting to pass the proposal
}

// Snapshot is the state of the authorization voting at a given point in time.
type Snapshot struct {
	Epoch uint64 // The number of blocks after which to checkpoint and reset the pending votes

	Number uint64                  // Block number where the snapshot was created
	Hash   types.Hash              // Block hash where the snapshot was created
	Votes  []*Vote                 // List of votes cast in chronological order
	Tally  map[types.Address]Tally // Current vote tally to avoid recalculating
	ValSet ibft.ValidatorSet       // Set of authorized validators at this moment
}

// newSnapshot create a new snapshot with the specified startup parameters. This
// method does not initialize the set of recent validators, so only ever use if for
// the genesis block.
func newSnapshot(epoch uint64, number uint64, hash types.Hash, valSet ibft.ValidatorSet) *Snapshot {
	snap := &Snapshot{
		Epoch:  epoch,
		Number: number,
		Hash:   hash,
		ValSet: valSet,
		Tally:  make(map[types.Address]Tally),
	}
	return snap
}

// loadSnapshot loads an existing snapshot from the database.
func loadSnapshot(epoch uint64, db storage.Storage, hash types.Hash) (*Snapshot, error) {
	blob, ok := db.ReadSnapshot(hash)
	if !ok {
		return nil, fmt.Errorf("snapshot not found")
	}
	snap := new(Snapshot)
	if err := json.Unmarshal(blob, snap); err != nil {
		return nil, err
	}
	snap.Epoch = epoch

	return snap, nil
}

// store inserts the snapshot into the database.
func (s *Snapshot) store(db storage.Storage) error {
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.WriteSnapshot(s.Hash, blob)
}

// copy creates a deep copy of the snapshot, though not the individual votes.
func (s *Snapshot) copy() *Snapshot {
	cpy := &Snapshot{
		Epoch:  s.Epoch,
		Number: s.Number,
		Hash:   s.Hash,
		ValSet: s.ValSet.Copy(),
		Votes:  make([]*Vote, len(s.Votes)),
		Tally:  make(map[types.Address]Tally),
	}

	for address, tally := range s.Tally {
		cpy.Tally[address] = tally
	}
	copy(cpy.Votes, s.Votes)

	return cpy
}

// checkVote return whether it's a valid vote
func (s *Snapshot) checkVote(address types.Address, authorize bool) bool {
	_, validator := s.ValSet.GetByAddress(address)
	return (validator != nil && !authorize) || (validator == nil && authorize)
}

// cast adds a new vote into the tally.
func (s *Snapshot) cast(address types.Address, authorize bool) bool {
	// Ensure the vote is meaningful
	if !s.checkVote(address, authorize) {
		return false
	}
	// Cast the vote into an existing or new tally
	if old, ok := s.Tally[address]; ok {
		old.Votes++
		s.Tally[address] = old
	} else {
		s.Tally[address] = Tally{Authorize: authorize, Votes: 1}
	}
	return true
}

// uncast removes a previously cast vote from the tally.
func (s *Snapshot) uncast(address types.Address, authorize bool) bool {
	// If there's no tally, it's a dangling vote, just drop
	tally, ok := s.Tally[address]
	if !ok {
		return false
	}
	// Ensure we only revert counted votes
	if tally.Authorize != authorize {
		return false
	}
	// Otherwise revert the vote
	if tally.Votes > 1 {
		tally.Votes--
		s.Tally[address] = tally
	} else {
		delete(s.Tally, address)
	}
	return true
}

// apply creates a new authorization snapshot by applying the given headers to
// the original one.
func (s *Snapshot) apply(headers []*types.Header) (*Snapshot, error) {
	// Allow passing in no headers for cleaner code
	if len(headers) == 0 {
		return s, nil
	}
	// Sanity check that the headers can be applied
	for i := 0; i < len(headers)-1; i++ {
		if headers[i+1].Number != headers[i].Number+1 {
			return nil, errInvalidVotingChain
		}
	}
	if headers[0].Number != s.Number+1 {
		return nil, errInvalidVotingChain
	}
	// Iterate through the headers and create a new snapshot
	snap := s.copy()

	for _, header := range headers {
		// Remove any votes on checkpoint blocks
		number := header.Number
		if number%s.Epoch == 0 {
			snap.Votes = nil
			snap.Tally = make(map[types.Address]Tally)
		}
		// Resolve the authorization key and check against validators
		validator, err := ecrecover(header)
		if err != nil {
			return nil, err
		}
		if _, v := snap.ValSet.GetByAddress(validator); v == nil {
			return nil, errUnauthorized
		}

		// Header authorized, discard any previous votes from the validator
		for i, vote := range snap.Votes {
			if vote.Validator == validator && vote.Address == header.Miner {
				// Uncast the vote from the cached tally
				snap.uncast(vote.Address, vote.Authorize)

				// Uncast the vote from the chronological list
				snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)
				break // only one vote allowed
			}
		}
		// Tally up the new vote from the validator
		var authorize bool
		switch {
		case bytes.Equal(header.Nonce[:], nonceAuthVote):
			authorize = true
		case bytes.Equal(header.Nonce[:], nonceDropVote):
			authorize = false
		default:
			return nil, errInvalidVote
		}
		if snap.cast(header.Miner, authorize) {
			snap.Votes = append(snap.Votes, &Vote{
				Validator: validator,
				Block:     number,
				Address:   header.Miner,
				Authorize: authorize,
			})
		}
		// If the vote passed, update the list of validators
		if tally := snap.Tally[header.Miner]; tally.Votes > snap.ValSet.Size()/2 {
			if tally.Authorize {
				snap.ValSet.AddValidator(header.Miner)
			} else {
				snap.ValSet.RemoveValidator(header.Miner)

				// Discard any previous votes the deauthorized validator cast
				for i := 0; i < len(snap.Votes); i++ {
					if snap.Votes[i].Validator == header.Miner {
						// Uncast the vote from the cached tally
						snap.uncast(snap.Votes[i].Address, snap.Votes[i].Authorize)

						// Uncast the vote from the chronological list
						snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)

						i--
					}
				}
			}
			// Discard any previous votes around the just changed account
			for i := 0; i < len(snap.Votes); i++ {
				if snap.Votes[i].Address == header.Miner {
					snap.Votes = append(snap.Votes[:i], snap.Votes[i+1:]...)
					i--
				}
			}
			delete(snap.Tally, header.Miner)
		}
	}
	snap.Number += uint64(len(headers))
	snap.Hash = headers[len(headers)-1].Hash

	return snap, nil
}

// validators retrieves the list of authorized validators in ascending order.
func (s *Snapshot) validators() []types.Address {
	validators := make([]types.Address, 0, s.ValSet.Size())
	for _, validator := range s.ValSet.List() {
		validators = append(validators, validator.Address())
	}
	for i := 0; i < len(validators); i++ {
		for j := i + 1; j < len(validators); j++ {
			if bytes.Compare(validators[i][:], validators[j][:]) > 0 {
				validators[i], validators[j] = validators[j], validators[i]
			}
		}
	}
	return validators
}

type snapshotJSON struct {
	Epoch  uint64                  `json:"epoch"`
	Number uint64                  `json:"number"`
	Hash   types.Hash              `json:"hash"`
	Votes  []*Vote                 `json:"votes"`
	Tally  map[types.Address]Tally `json:"tally"`

	// for validator set
	Validators []types.Address     `json:"validators"`
	Policy     ibft.ProposerPolicy `json:"policy"`
}

func (s *Snapshot) toJSONStruct() *snapshotJSON {
	return &snapshotJSON{
		Epoch:      s.Epoch,
		Number:     s.Number,
		Hash:       s.Hash,
		Votes:      s.Votes,
		Tally:      s.Tally,
		Validators: s.validators(),
		Policy:     s.ValSet.Policy(),
	}
}

// Unmarshal from a json byte array
func (s *Snapshot) UnmarshalJSON(b []byte) error {
	var j snapshotJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}

	s.Epoch = j.Epoch
	s.Number = j.Number
	s.Hash = j.Hash
	s.Votes = j.Votes
	s.Tally = j.Tally
	s.ValSet = validator.NewSet(j.Validators, j.Policy)
	return nil
}

// Marshal to a json byte array
func (s *Snapshot) MarshalJSON() ([]byte, error) {
	j := s.toJSONStruct()
	return json.Marshal(j)
}
