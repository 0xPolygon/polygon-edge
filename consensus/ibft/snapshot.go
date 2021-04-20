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

func (i *Ibft) setupSnapshot() error {
	i.store = newSnapshotStore()

	// read from storage
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
		// add genesis
		if err := i.addHeaderSnap(header); err != nil {
			return err
		}
	}

	// some of the data might get lost due to ungrateful disconnections
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

func (i *Ibft) addHeaderSnap(header *types.Header) error {
	// genesis header needs to be set by hand, all the other
	// snapshots are set as part of processHeaders
	extra, err := getIbftExtra(header)
	if err != nil {
		return err
	}

	// create the first snapshot from the genesis
	snap := &Snapshot{
		Hash:   header.Hash.String(),
		Number: header.Number,
		Votes:  []*Vote{},
		Set:    extra.Validators,
	}
	i.store.add(snap)
	return nil
}

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

func (i *Ibft) saveSnapDataToFile() error {
	if i.config.Path == "" {
		return nil
	}
	return i.store.saveToPath(i.config.Path)
}

func (i *Ibft) processHeaders(headers []*types.Header) error {
	if len(headers) == 0 {
		return nil
	}

	parentSnap, err := i.getSnapshot(headers[0].Number - 1)
	if err != nil {
		return err
	}
	snap := parentSnap.Copy()

	saveSnap := func(h *types.Header) error {
		if snap.Equal(parentSnap) {
			return nil
		}

		snap.Number = h.Number
		snap.Hash = h.Hash.String()

		i.store.add(snap)

		parentSnap = snap
		snap = parentSnap.Copy()
		return nil
	}

	for _, h := range headers {
		number := h.Number

		validator, err := ecrecoverFromHeader(h)
		if err != nil {
			return err
		}
		if !snap.Set.Includes(validator) {
			return fmt.Errorf("unauthroized validator")
		}

		if number%i.epochSize == 0 {
			// during a checkpoint block, we reset the voles
			// and there cannot be any proposals
			snap.Votes = nil
			if err := saveSnap(h); err != nil {
				return err
			}

			// remove in-memory snaphots from two epochs before this one
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

		count := snap.Count(func(v *Vote) bool {
			return v.Validator == validator && v.Address == h.Miner
		})
		if count > 1 {
			// there can only be one vote per validator per address
			return fmt.Errorf("more than one proposal per validator per address found")
		}
		if count == 0 {
			// cast the new vote since there is no one yet
			snap.Votes = append(snap.Votes, &Vote{
				Validator: validator,
				Address:   h.Miner,
				Authorize: authorize,
			})
		}

		// check the tally for the proposed validator
		tally := snap.Count(func(v *Vote) bool {
			return v.Address == h.Miner
		})

		if tally > snap.Set.Len()/2 {
			if authorize {
				// add the proposal to the validator list
				snap.Set.Add(h.Miner)
			} else {
				// remove the proposal from the validators list
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

		if err := saveSnap(h); err != nil {
			return nil
		}
	}

	// update the metadata
	i.store.updateLastBlock(headers[len(headers)-1].Number)
	return nil
}

func (i *Ibft) getSnapshotMetadata() (*snapshotMetadata, error) {
	meta := &snapshotMetadata{
		LastBlock: i.store.getLastBlock(),
	}
	return meta, nil
}

func (i *Ibft) getSnapshot(num uint64) (*Snapshot, error) {
	snap := i.store.find(num)
	return snap, nil
}

type Vote struct {
	Validator types.Address
	Address   types.Address
	Authorize bool
}

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

type snapshotMetadata struct {
	LastBlock uint64
}

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

func (s *Snapshot) Count(h func(v *Vote) bool) (count int) {
	for _, v := range s.Votes {
		if h(v) {
			count++
		}
	}
	return
}

func (s *Snapshot) RemoveVotes(h func(v *Vote) bool) {
	for i := 0; i < len(s.Votes); i++ {
		if h(s.Votes[i]) {
			s.Votes = append(s.Votes[:i], s.Votes[i+1:]...)
			i--
		}
	}
}

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

type snapshotStore struct {
	lastNumber uint64
	lock       sync.Mutex
	list       snapshotSortedList
}

func newSnapshotStore() *snapshotStore {
	return &snapshotStore{
		list: snapshotSortedList{},
	}
}

func (s *snapshotStore) loadFromPath(path string) error {
	// load metadata
	var meta *snapshotMetadata
	if err := readDataStore(filepath.Join(path, "metadata"), &meta); err != nil {
		return err
	}
	if meta != nil {
		s.lastNumber = meta.LastBlock
	}

	// load snapshots
	snaps := []*Snapshot{}
	if err := readDataStore(filepath.Join(path, "snapshots"), &snaps); err != nil {
		return err
	}
	for _, snap := range snaps {
		s.add(snap)
	}
	return nil
}

func (s *snapshotStore) saveToPath(path string) error {
	// write snapshots
	if err := writeDataStore(filepath.Join(path, "snapshots"), s.list); err != nil {
		return err
	}

	// write metadata
	meta := &snapshotMetadata{
		LastBlock: s.lastNumber,
	}
	if err := writeDataStore(filepath.Join(path, "metadata"), meta); err != nil {
		return err
	}
	return nil
}

func (s *snapshotStore) getLastBlock() uint64 {
	return atomic.LoadUint64(&s.lastNumber)
}

func (s *snapshotStore) updateLastBlock(num uint64) {
	atomic.StoreUint64(&s.lastNumber, num)
}

func (s *snapshotStore) deleteLower(num uint64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	i := sort.Search(len(s.list), func(i int) bool {
		return s.list[i].Number >= num
	})
	s.list = s.list[i:]
}

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

func (s *snapshotStore) add(snap *Snapshot) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// append and sort the list
	s.list = append(s.list, snap)
	sort.Sort(&s.list)
}

type snapshotSortedList []*Snapshot

func (s snapshotSortedList) Len() int {
	return len(s)
}

func (s snapshotSortedList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s snapshotSortedList) Less(i, j int) bool {
	return s[i].Number < s[j].Number
}

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
