package ibft2

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
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

func (i *Ibft2) closeSnapshot() error {
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
	data, err := json.Marshal(snaps)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(i.snapshotsFilePath(), data, 0755); err != nil {
		return err
	}
	return nil
}

func (i *Ibft2) snapshotsFilePath() string {
	return filepath.Join(i.config.Path, "snapshots")
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

type ValidatorSet []types.Address

func (v *ValidatorSet) CalcProposer(round uint64, lastProposer types.Address) types.Address {
	seed := uint64(0)
	if lastProposer == types.ZeroAddress {
		seed = round
	} else {
		offset := 0
		if indx := v.Index(lastProposer); indx != -1 {
			offset = indx
		}
		seed = uint64(offset) + round + 1
	}
	pick := seed % uint64(v.Len())
	return (*v)[pick]
}

func (v *ValidatorSet) Add(addr types.Address) {
	*v = append(*v, addr)
}

func (v *ValidatorSet) Del(addr types.Address) {
	for indx, i := range *v {
		if i == addr {
			*v = append((*v)[:indx], (*v)[indx+1:]...)
		}
	}
}

func (v *ValidatorSet) Len() int {
	return len(*v)
}

func (v *ValidatorSet) Equal(vv *ValidatorSet) bool {
	if len(*v) != len(*vv) {
		return false
	}
	for indx := range *v {
		if (*v)[indx] != (*vv)[indx] {
			return false
		}
	}
	return true
}

func (v *ValidatorSet) Index(addr types.Address) int {
	for indx, i := range *v {
		if i == addr {
			return indx
		}
	}
	return -1
}

func (v *ValidatorSet) Includes(addr types.Address) bool {
	return v.Index(addr) != -1
}

func (v *ValidatorSet) MinFaultyNodes() int {
	return int(math.Ceil(float64(len(*v))/3)) - 1
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
