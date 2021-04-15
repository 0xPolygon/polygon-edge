package ibft2

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-memdb"
)

var (
	// Magic nonce number to vote on adding a new validator
	nonceAuthVote = types.Nonce{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	// Magic nonce number to vote on removing a validator.
	nonceDropVote = types.Nonce{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
)

func (i *Ibft2) setupSnapshot() error {
	store, err := memdb.NewMemDB(schema)
	if err != nil {
		return err
	}
	i.store = store

	// read from storage
	if err := i.loadSnapDataFromFile(); err != nil {
		return err
	}

	header := i.blockchain.Header()
	meta, err := i.getSnapshotMetadata()
	if err != nil {
		return err
	}

	if header.Number == 0 || meta.LastBlock == 0 {
		extra, err := getIbftExtra(header)
		if err != nil {
			return err
		}

		// create the first snapshot from the genesis
		snap := &Snapshot{
			Hash:   header.Hash.String(),
			Number: 0,
			Votes:  []*Vote{},
			Set:    extra.Validators,
		}
		if err := i.putSnapshot(snap); err != nil {
			return err
		}
	}

	// some of the data might get lost due to ungrateful disconnections
	if header.Number > meta.LastBlock {
		i.logger.Info("syncing past snapshots", "from", meta.LastBlock, "to", header.Number)

		for num := meta.LastBlock + 1; num <= header.Number; num++ {
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

func (i *Ibft2) getLatestSnapshot() (*Snapshot, error) {
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

func (i *Ibft2) loadSnapDataFromFile() error {
	if i.config.Path == "" {
		return nil
	}

	// load metadata
	var meta *snapshotMetadata
	if err := i.readDataStore("metadata", &meta); err != nil {
		return err
	}
	if meta != nil {
		if err := i.updateSnapshotMetadata(meta.LastBlock); err != nil {
			return err
		}
	}

	// load snapshots
	snaps := []*Snapshot{}
	if err := i.readDataStore("snapshots", &snaps); err != nil {
		return err
	}
	for _, s := range snaps {
		if err := i.putSnapshot(s); err != nil {
			return err
		}
	}
	return nil
}

func (i *Ibft2) saveSnapDataToFile() error {
	if i.config.Path == "" {
		return nil
	}

	meta, err := i.getSnapshotMetadata()
	if err != nil {
		return err
	}

	txn := i.store.Txn(false)

	// List all the snapshots
	it, err := txn.Get("snapshot", "number")
	if err != nil {
		return err
	}

	snaps := []*Snapshot{}
	for obj := it.Next(); obj != nil; obj = it.Next() {
		snaps = append(snaps, obj.(*Snapshot))
	}
	if err := i.writeDataStore("snapshots", snaps); err != nil {
		return err
	}

	// write metadata
	if err := i.writeDataStore("metadata", meta); err != nil {
		return err
	}
	return nil
}

func (i *Ibft2) readDataStore(file string, obj interface{}) error {
	path := filepath.Join(i.config.Path, file)
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

func (i *Ibft2) writeDataStore(file string, obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(i.config.Path, file), data, 0755); err != nil {
		return err
	}
	return nil
}

func (i *Ibft2) processHeaders(headers []*types.Header) error {
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

		snap.Number = int(h.Number)
		snap.Hash = h.Hash.String()
		if err := i.putSnapshot(snap); err != nil {
			return err
		}

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
				if err := i.removeSnapshotsLowerThan(purgeBlock); err != nil {
					return err
				}
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
	if err := i.updateSnapshotMetadata(headers[len(headers)-1].Number); err != nil {
		return err
	}
	return nil
}

func (i *Ibft2) removeSnapshotsLowerThan(num uint64) error {
	txn := i.store.Txn(true)

	it, err := txn.ReverseLowerBound("snapshot", "number", int(num))
	if err != nil {
		return err
	}

	for obj := it.Next(); obj != nil; obj = it.Next() {
		if err := txn.Delete("snapshot", obj); err != nil {
			return err
		}
	}
	txn.Commit()
	return nil
}

func (i *Ibft2) updateSnapshotMetadata(num uint64) error {
	txn := i.store.Txn(true)
	meta := &snapshotMetadata{
		LastBlock: num,
	}
	if err := txn.Insert("meta", meta); err != nil {
		return err
	}
	txn.Commit()
	return nil
}

func (i *Ibft2) getSnapshotMetadata() (*snapshotMetadata, error) {
	txn := i.store.Txn(false)
	meta, err := txn.First("meta", "id")
	if err != nil {
		return nil, err
	}
	if meta == nil {
		meta = &snapshotMetadata{
			LastBlock: 0,
		}
	}
	return meta.(*snapshotMetadata), nil
}

func (i *Ibft2) putSnapshot(snap *Snapshot) error {
	txn := i.store.Txn(true)
	if err := txn.Insert("snapshot", snap); err != nil {
		return err
	}
	txn.Commit()
	return nil
}

func (i *Ibft2) getSnapshot(num uint64) (*Snapshot, error) {
	txn := i.store.Txn(false)
	defer txn.Abort()

	it, err := txn.ReverseLowerBound("snapshot", "number", int(num))
	if err != nil {
		return nil, err
	}
	return it.Next().(*Snapshot), nil
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
	Number int

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

// schema for the state
var schema *memdb.DBSchema

func init() {
	schema = &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			"snapshot": {
				Name: "snapshot",
				Indexes: map[string]*memdb.IndexSchema{
					"id": {
						Name:    "id",
						Unique:  true,
						Indexer: &memdb.StringFieldIndex{Field: "Hash"},
					},
					"number": {
						Name:    "number",
						Unique:  true,
						Indexer: &memdb.IntFieldIndex{Field: "Number"},
					},
				},
			},
			"meta": {
				Name: "meta",
				Indexes: map[string]*memdb.IndexSchema{
					"id": {
						Name:         "id",
						AllowMissing: false,
						Unique:       true,
						Indexer:      singletonRecord, // we store only 1 metadata record
					},
				},
			},
		},
	}
}

var singletonRecord = &memdb.ConditionalIndex{
	Conditional: func(interface{}) (bool, error) { return true, nil },
}
