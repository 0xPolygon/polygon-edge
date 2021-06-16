package ibft

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/types"
)

var (
	// Magic nonce number to vote on adding a new validator
	nonceAuthVote = types.Nonce{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	// Magic nonce number to vote on removing a validator.
	nonceDropVote = types.Nonce{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
)

// setupSnapshot sets up the snapshot store for the IBFT object
func (i *Ibft) setupSnapshot() error {
	i.store = newSnapshotStore()

	// Read from storage
	if i.config.Path != "" {
		if err := i.store.loadFromPath(i.config.Path); err != nil {
			return err
		}
	}

	header := i.blockchain.Header()
	meta, err := i.getSnapshotMetadata()
	if err != nil {
		return err
	}

	if header.Number == 0 {
		// Add genesis
		if err := i.addHeaderSnap(header); err != nil {
			return err
		}
	}

	// Some of the data might get lost due to ungrateful disconnections
	if header.Number > meta.LastBlock {
		i.logger.Info("syncing past snapshots", "from", meta.LastBlock, "to", header.Number)

		for num := meta.LastBlock + 1; num <= header.Number; num++ {
			if num == 0 {
				continue
			}
			header, ok := i.blockchain.GetHeaderByNumber(num)
			if !ok {
				return fmt.Errorf("header %d not found", num)
			}
			if err := i.processHeaders([]*types.Header{header}); err != nil {
				return err
			}
		}
	}

	return nil
}

// addHeaderSnap creates the initial snapshot, and adds it to the snapshot store
func (i *Ibft) addHeaderSnap(header *types.Header) error {
	// Genesis header needs to be set by hand, all the other
	// snapshots are set as part of processHeaders
	extra, err := getIbftExtra(header)
	if err != nil {
		return err
	}

	// Create the first snapshot from the genesis
	snap := &Snapshot{
		Hash:   header.Hash.String(),
		Number: header.Number,
		Votes:  []*Vote{},
		Set:    extra.Validators,
	}

	i.store.add(snap)

	return nil
}

// getLatestSnapshot returns the latest snapshot object
func (i *Ibft) getLatestSnapshot() (*Snapshot, error) {
	meta, err := i.getSnapshotMetadata()
	if err != nil {
		return nil, err
	}

	snap, err := i.getSnapshot(meta.LastBlock)
	if err != nil {
		return nil, err
	}

	return snap, nil
}

// processHeaders is the powerhouse method in the snapshot module.

// It processes passed in headers, and updates the snapshot / snapshot store
func (i *Ibft) processHeaders(headers []*types.Header) error {
	if len(headers) == 0 {
		return nil
	}

	parentSnap, err := i.getSnapshot(headers[0].Number - 1)
	if err != nil {
		return err
	}
	snap := parentSnap.Copy()

	// TODO: This is difficult to understand, and the error is unneeded
	// saveSnap is a callback function for saving the passed in header to the snapshot store
	saveSnap := func(h *types.Header) error {
		snap.Number = h.Number
		snap.Hash = h.Hash.String()

		i.store.add(snap)

		parentSnap = snap
		snap = parentSnap.Copy()

		return nil
	}

	for _, h := range headers {
		number := h.Number

		proposer, err := ecrecoverFromHeader(h)
		if err != nil {
			return err
		}

		// Check if the recovered proposer is part of the validator set
		if !snap.Set.Includes(proposer) {
			return fmt.Errorf("unauthorized proposer")
		}

		if number%i.epochSize == 0 {
			// during a checkpoint block, we reset the votes
			// and there cannot be any proposals
			snap.Votes = nil
			if err := saveSnap(h); err != nil {
				return err
			}

			// remove in-memory snapshots from two epochs before this one
			epoch := int(number/i.epochSize) - 2
			if epoch > 0 {
				purgeBlock := uint64(epoch) * i.epochSize
				i.store.deleteLower(purgeBlock)
			}
			continue
		}

		// if we have a miner address, this might be a vote
		if h.Miner == types.ZeroAddress {
			continue
		}

		// the nonce selects the action
		var authorize bool
		if h.Nonce == nonceAuthVote {
			authorize = true
		} else if h.Nonce == nonceDropVote {
			authorize = false
		} else {
			return fmt.Errorf("incorrect vote nonce")
		}

		// validate the vote
		if authorize {
			// we can only authorize if they are not on the validators list
			if snap.Set.Includes(h.Miner) {
				continue
			}
		} else {
			// we can only remove if they are part of the validators list
			if !snap.Set.Includes(h.Miner) {
				continue
			}
		}

		voteCount := snap.Count(func(v *Vote) bool {
			return v.Validator == proposer && v.Address == h.Miner
		})

		if voteCount > 1 {
			// there can only be one vote per validator per address
			return fmt.Errorf("more than one proposal per validator per address found")
		}
		if voteCount == 0 {
			// cast the new vote since there is no one yet
			snap.Votes = append(snap.Votes, &Vote{
				Validator: proposer,
				Address:   h.Miner,
				Authorize: authorize,
			})
		}

		// check the tally for the proposed validator
		tally := snap.Count(func(v *Vote) bool {
			return v.Address == h.Miner
		})

		// If more than a half of all validators voted
		if tally > snap.Set.Len()/2 {
			if authorize {
				// add the candidate to the validators list
				snap.Set.Add(h.Miner)
			} else {
				// remove the candidate from the validators list
				snap.Set.Del(h.Miner)

				// remove any votes casted by the removed validator
				snap.RemoveVotes(func(v *Vote) bool {
					return v.Validator == h.Miner
				})
			}

			// remove all the votes that promoted this validator
			snap.RemoveVotes(func(v *Vote) bool {
				return v.Address == h.Miner
			})
		}

		if !snap.Equal(parentSnap) {
			if err := saveSnap(h); err != nil {
				return nil
			}
		}
	}

	// update the metadata
	i.store.updateLastBlock(headers[len(headers)-1].Number)

	return nil
}

// getSnapshotMetadata returns the latest snapshot metadata
func (i *Ibft) getSnapshotMetadata() (*snapshotMetadata, error) {
	meta := &snapshotMetadata{
		LastBlock: i.store.getLastBlock(),
	}

	return meta, nil
}

// getSnapshot returns the snapshot at the specified block height
func (i *Ibft) getSnapshot(num uint64) (*Snapshot, error) {
	snap := i.store.find(num)

	return snap, nil
}

// Vote defines the vote structure
type Vote struct {
	Validator types.Address
	Address   types.Address
	Authorize bool
}

// Equal checks if two votes are equal
func (v *Vote) Equal(vv *Vote) bool {
	if v.Validator != vv.Validator {
		return false
	}

	if v.Address != vv.Address {
		return false
	}

	if v.Authorize != vv.Authorize {
		return false
	}

	return true
}

// Copy makes a copy of the vote, and returns it
func (v *Vote) Copy() *Vote {
	vv := new(Vote)
	*vv = *v
	return vv
}

// Snapshot is the current state at a given point in time for validators and votes
type Snapshot struct {
	// block number when the snapshot was created
	Number uint64

	// block hash when the snapshot was created
	Hash string

	// votes casted in chronological order
	Votes []*Vote

	// current set of validators
	Set ValidatorSet
}

// snapshotMetadata defines the metadata for the snapshot
type snapshotMetadata struct {
	// LastBlock represents the latest block in the snapshot
	LastBlock uint64
}

// Equal checks if two snapshots are equal
func (s *Snapshot) Equal(ss *Snapshot) bool {
	// we only check if Votes and Set are equal since Number and Hash
	// are only meant to be used for indexing
	if len(s.Votes) != len(ss.Votes) {
		return false
	}
	for indx := range s.Votes {
		if !s.Votes[indx].Equal(ss.Votes[indx]) {
			return false
		}
	}
	if !s.Set.Equal(&ss.Set) {
		return false
	}
	return true
}

// Count returns the vote tally.
// The count increases if the callback function returns true
func (s *Snapshot) Count(h func(v *Vote) bool) (count int) {
	for _, v := range s.Votes {
		if h(v) {
			count++
		}
	}
	return
}

// RemoveVotes removes votes from the snapshot, based on the passed in callback
func (s *Snapshot) RemoveVotes(h func(v *Vote) bool) {
	for i := 0; i < len(s.Votes); i++ {
		if h(s.Votes[i]) {
			s.Votes = append(s.Votes[:i], s.Votes[i+1:]...)
			i--
		}
	}
}

// Copy makes a copy of the snapshot
func (s *Snapshot) Copy() *Snapshot {
	// Do not need to copy Number and Hash
	ss := &Snapshot{
		Votes: make([]*Vote, len(s.Votes)),
		Set:   ValidatorSet{},
	}

	for indx, vote := range s.Votes {
		ss.Votes[indx] = vote.Copy()
	}

	ss.Set = append(ss.Set, s.Set...)

	return ss
}

// ToProto converts the snapshot to a Proto snapshot
func (s *Snapshot) ToProto() *proto.Snapshot {
	resp := &proto.Snapshot{
		Validators: []*proto.Snapshot_Validator{},
		Votes:      []*proto.Snapshot_Vote{},
		Number:     uint64(s.Number),
		Hash:       s.Hash,
	}

	// add votes
	for _, vote := range s.Votes {
		resp.Votes = append(resp.Votes, &proto.Snapshot_Vote{
			Validator: vote.Validator.String(),
			Proposed:  vote.Address.String(),
			Auth:      vote.Authorize,
		})
	}

	// add addresses
	for _, val := range s.Set {
		resp.Validators = append(resp.Validators, &proto.Snapshot_Validator{
			Address: val.String(),
		})
	}

	return resp
}

// snapshotStore defines the structure of the stored snapshots
type snapshotStore struct {
	// lastNumber is the latest block number stored
	lastNumber uint64

	// lock is the snapshotStore mutex
	lock sync.Mutex

	// list represents the actual snapshot sorted list
	list snapshotSortedList
}

// newSnapshotStore returns a new snapshot store
func newSnapshotStore() *snapshotStore {
	return &snapshotStore{
		list: snapshotSortedList{},
	}
}

// loadFromPath loads a saved snapshot store from the specified file system path
func (s *snapshotStore) loadFromPath(path string) error {
	// Load metadata
	var meta *snapshotMetadata
	if err := readDataStore(filepath.Join(path, "metadata"), &meta); err != nil {
		return err
	}
	if meta != nil {
		s.lastNumber = meta.LastBlock
	}

	// Load snapshots
	snaps := []*Snapshot{}
	if err := readDataStore(filepath.Join(path, "snapshots"), &snaps); err != nil {
		return err
	}
	for _, snap := range snaps {
		s.add(snap)
	}

	return nil
}

// saveToPath saves the snapshot store as a file to the specified path
func (s *snapshotStore) saveToPath(path string) error {
	// Write snapshots
	if err := writeDataStore(filepath.Join(path, "snapshots"), s.list); err != nil {
		return err
	}

	// Write metadata
	meta := &snapshotMetadata{
		LastBlock: s.lastNumber,
	}
	if err := writeDataStore(filepath.Join(path, "metadata"), meta); err != nil {
		return err
	}

	return nil
}

// getLastBlock returns the latest block number from the snapshot store. [Thread safe]
func (s *snapshotStore) getLastBlock() uint64 {
	return atomic.LoadUint64(&s.lastNumber)
}

// updateLastBlock sets the latest block number in the snapshot store. [Thread safe]
func (s *snapshotStore) updateLastBlock(num uint64) {
	atomic.StoreUint64(&s.lastNumber, num)
}

// deleteLower deletes snapshots that have a block number lower than the passed in parameter
func (s *snapshotStore) deleteLower(num uint64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	i := sort.Search(len(s.list), func(i int) bool {
		return s.list[i].Number >= num
	})
	s.list = s.list[i:]
}

// find returns the index of the first closest snapshot to the number specified
func (s *snapshotStore) find(num uint64) *Snapshot {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.list) == 0 {
		return nil
	}

	// fast track, check the last item
	if last := s.list[len(s.list)-1]; last.Number < num {
		return last
	}

	i := sort.Search(len(s.list), func(i int) bool {
		return s.list[i].Number >= num
	})

	if i < len(s.list) {
		if i == 0 {
			return s.list[0]
		}

		if s.list[i].Number == num {
			return s.list[i]
		}

		return s.list[i-1]
	}

	return nil
}

// add adds a new snapshot to the snapshot store
func (s *snapshotStore) add(snap *Snapshot) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// append and sort the list
	s.list = append(s.list, snap)
	sort.Sort(&s.list)
}

// snapshotSortedList defines the sorted snapshot list
type snapshotSortedList []*Snapshot

// Len returns the size of the sorted snapshot list
func (s snapshotSortedList) Len() int {
	return len(s)
}

// Swap swaps two values in the sorted snapshot list
func (s snapshotSortedList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less checks if the element at index I has a lower number than the element at index J
func (s snapshotSortedList) Less(i, j int) bool {
	return s[i].Number < s[j].Number
}

// readDataStore attempts to read the specific file from file storage
func readDataStore(path string, obj interface{}) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, obj); err != nil {
		return err
	}

	return nil
}

// writeDataStore attempts to write the specific file to file storage
func writeDataStore(path string, obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(path, data, 0755); err != nil {
		return err
	}

	return nil
}
