package ibft

import (
	"fmt"
	"math"

	"github.com/0xPolygon/minimal/consensus/ibft/aux"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-memdb"
)

var (
	// Magic nonce number to vote on adding a new validator
	nonceAuthVote = hex.MustDecodeHex("0xffffffffffffffff")

	// Magic nonce number to vote on removing a validator.
	nonceDropVote = hex.MustDecodeHex("0x0000000000000000")
)

type snapshotState struct {
	store   aux.BlockchainInterface
	config  *Config
	readyCh chan struct{}
	db      *memdb.MemDB
}

func (s *snapshotState) setup() error {
	db, err := memdb.NewMemDB(schema)
	if err != nil {
		return err
	}
	s.db = db

	// create the first snapshot for the genesis
	genesis, ok := s.store.GetHeaderByNumber(0)
	if !ok {
		return fmt.Errorf("failed to get genesis")
	}

	fmt.Println("- extra -")
	fmt.Println(genesis.ExtraData)

	extra, err := getIbftExtra(genesis)
	if err != nil {
		return err
	}
	fmt.Println(extra.Validators)
	genesisSnap := &Snapshot2{
		Number: 0,
		Hash:   genesis.Hash.String(),
		Votes:  []*Vote2{},
		Set:    extra.Validators,
	}
	txn := s.db.Txn(true)
	if err := s.writeSnapshot(txn, genesisSnap); err != nil {
		return err
	}
	txn.Commit()

	return nil
}

func (s *snapshotState) verifyHeader(header *types.Header) error {
	snap, err := s.getSnapshot(int(header.Number) - 1)
	if err != nil {
		return err
	}
	signer, err := ecrecover(header)
	if err != nil {
		return err
	}
	if !snap.Set.Includes(signer) {
		return fmt.Errorf("not authorized")
	}

	// validate committed seals
	extra, err := getIbftExtra(header)
	if err != nil {
		return err
	}
	if len(extra.CommittedSeal) == 0 {
		return fmt.Errorf("cannot be zero")
	}

	msg := commitMsg(header.Hash)

	found := map[types.Address]struct{}{}
	for _, seal := range extra.CommittedSeal {
		addr, err := ecrecoverImpl(seal, msg)
		if err != nil {
			return err
		}

		if _, ok := found[addr]; ok {
			return fmt.Errorf("repeated")
		} else if !snap.Set.Includes(addr) {
			return fmt.Errorf("not found in set")
		} else {
			found[addr] = struct{}{}
		}
	}

	if len(found) <= 2*snap.Set.MinFaultyNodes() {
		return fmt.Errorf("not enough healthy nodes")
	}

	// TODO: if the header is valid, update the snapshot
	return nil
}

func (s *snapshotState) getSnapshot(num int) (*Snapshot2, error) {
	txn := s.db.Txn(false)
	defer txn.Abort()

	it, err := txn.ReverseLowerBound("snapshot", "number", 20)
	if err != nil {
		return nil, err
	}
	return it.Next().(*Snapshot2), nil
}

func (s *snapshotState) writeSnapshot(txn *memdb.Txn, snap *Snapshot2) error {
	if err := txn.Insert("snapshot", snap); err != nil {
		return err
	}
	return nil
}

func (s *snapshotState) processHeaders(headers []*types.Header) error {

	// make sure there is a sequence in the headers
	for i := 1; i < len(headers); i++ {
		if headers[i-1].Number != headers[i].Number-1 {
			return fmt.Errorf("bad sequence")
		}
		if headers[i-1].Hash != headers[i].ParentHash {
			return fmt.Errorf("bad hash sequence")
		}
	}

	// get the latest snapshot
	snapshot, err := s.getSnapshot(int(headers[0].Number))
	if err != nil {
		return err
	}
	fmt.Println("-- snapshot --")
	fmt.Println(snapshot)

	currentSnapshot := snapshot.Copy()
	fmt.Println(currentSnapshot)

	for _, header := range headers {
		number := header.Number
		if number%s.config.Epoch == 0 {
			currentSnapshot.Votes = nil
		}

		validator, err := ecrecover(header)
		if err != nil {
			return err
		}
		if !currentSnapshot.Set.Includes(validator) {
			return fmt.Errorf("bad")
		}

		fmt.Println("-- validator --")
		fmt.Println(validator)

	}

	return nil
}

func (s *snapshotState) start() {
	// create the schemas
	// Create a new data base
	db, err := memdb.NewMemDB(schema)
	if err != nil {
		panic(err)
	}

	// Create a write transaction

	// Insert some people
	people := []*Snapshot2{
		{Hash: "joe@aol.com", Number: 30},
		{Hash: "lucy@aol.com", Number: 25},
		{Hash: "tariq@aol.com", Number: 26},
		{Hash: "dorothy@aol.com", Number: 53},
	}

	for _, p := range people {
		txn := db.Txn(true)
		if err := txn.Insert("snapshot", p); err != nil {
			panic(err)
		}
		txn.Commit()
	}

	// Commit the transaction

	// Create read-only transaction
	txn := db.Txn(false)
	defer txn.Abort()

	// Range scan over people with ages between 25 and 35 inclusive
	it, err := txn.LowerBound("snapshot", "number", 25)
	if err != nil {
		panic(err)
	}

	fmt.Println("People aged 25 - 35:")
	fmt.Println(it.Next().(*Snapshot2))

}

/*
type Validator2 struct {
	Address types.Address
}

func (v *Validator2) Copy() *Validator2 {
	vv := new(Validator2)
	*vv = *v
	return vv
}
*/

type ValidatorSet2 []types.Address

func (v *ValidatorSet2) Includes(addr types.Address) bool {
	for _, i := range *v {
		if i == addr {
			return true
		}
	}
	return false
}

func (v *ValidatorSet2) MinFaultyNodes() int {
	return int(math.Ceil(float64(len(*v))/3)) - 1
}

type Vote2 struct {
	Validator types.Address
	Block     uint64
	Address   types.Address
	Authorize bool
}

func (v *Vote2) Copy() *Vote2 {
	vv := new(Vote2)
	*vv = *v
	return vv
}

type Tally2 struct {
	Authorize bool
	Votes     int
}

// Snapshot2 is the current state at a given point in time for validators and votes
type Snapshot2 struct {
	// block number when the snapshot was created
	Number int

	// block hash when the snapshot was created
	Hash string

	// votes casted in chronological order
	Votes []*Vote2

	// current vote tally
	// Tally map[types.Address]*Tally2

	// current set of validators
	Set ValidatorSet2
}

func (s *Snapshot2) NextValidator(last types.Address) types.Address {
	if last == types.ZeroAddress {
		// pick the first one
		return s.Set[0]
	}
	// find the address of the last validator and pick the next one
	addrIndx := -1
	for indx, addr := range s.Set {
		if addr == last {
			addrIndx = indx
		}
	}
	if addrIndx == -1 {
		// if nothing found, return the first one
		return s.Set[0]
	}
	if addrIndx == len(s.Set)-1 {
		// if the address is the last one, pick the first one
		return s.Set[0]
	}
	return s.Set[addrIndx+1]
}

func (s *Snapshot2) Copy() *Snapshot2 {
	ss := new(Snapshot2)
	*ss = *s

	ss.Votes = make([]*Vote2, len(s.Votes))
	for indx, vote := range s.Votes {
		ss.Votes[indx] = vote.Copy()
	}

	ss.Set = make(ValidatorSet2, len(s.Set))
	for indx, val := range s.Set {
		ss.Set[indx] = val
	}
	return ss
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
		},
	}
}
