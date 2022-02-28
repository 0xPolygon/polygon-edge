package ibft

import (
	"crypto/ecdsa"
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

// initIbftMechanism initializes the IBFT mechanism for unit tests
func initIbftMechanism(mechanismType MechanismType, ibft *Ibft) {
	mechanismFactory := mechanismBackends[mechanismType]
	mechanism, _ := mechanismFactory(ibft, &IBFTFork{
		Type: mechanismType,
		From: common.JSONNumber{Value: 0},
	})
	ibft.mechanisms = []ConsensusMechanism{mechanism}
}

func getTempDir(t *testing.T) string {
	t.Helper()

	tmpDir, err := ioutil.TempDir("/tmp", "snapshot-store")
	assert.NoError(t, err)
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Error(err)
		}
	})

	return tmpDir
}

type testerAccount struct {
	alias string
	priv  *ecdsa.PrivateKey
}

func (t *testerAccount) Address() types.Address {
	return crypto.PubKeyToAddress(&t.priv.PublicKey)
}

func (t *testerAccount) sign(h *types.Header) *types.Header {
	h, _ = writeSeal(t.priv, h)

	return h
}

type testerAccountPool struct {
	accounts []*testerAccount
}

func newTesterAccountPool(num ...int) *testerAccountPool {
	t := &testerAccountPool{
		accounts: []*testerAccount{},
	}

	if len(num) == 1 {
		for i := 0; i < num[0]; i++ {
			key, _ := crypto.GenerateKey()

			t.accounts = append(t.accounts, &testerAccount{
				alias: strconv.Itoa(i),
				priv:  key,
			})
		}
	}

	return t
}

func (ap *testerAccountPool) add(accounts ...string) {
	for _, account := range accounts {
		if acct := ap.get(account); acct != nil {
			continue
		}

		priv, err := crypto.GenerateKey()
		if err != nil {
			panic("BUG: Failed to generate crypto key")
		}

		ap.accounts = append(ap.accounts, &testerAccount{
			alias: account,
			priv:  priv,
		})
	}
}

func (ap *testerAccountPool) genesis() *chain.Genesis {
	genesis := &types.Header{
		MixHash: IstanbulDigest,
	}
	putIbftExtraValidators(genesis, ap.ValidatorSet())
	genesis.ComputeHash()

	c := &chain.Genesis{
		Mixhash:   genesis.MixHash,
		ExtraData: genesis.ExtraData,
	}

	return c
}

func (ap *testerAccountPool) get(name string) *testerAccount {
	for _, i := range ap.accounts {
		if i.alias == name {
			return i
		}
	}

	return nil
}

func (ap *testerAccountPool) ValidatorSet() ValidatorSet {
	v := ValidatorSet{}
	for _, i := range ap.accounts {
		v = append(v, i.Address())
	}

	return v
}

type mockVote struct {
	validator string
	candidate string
	auth      bool
}

func skipVote(validator string) mockVote {
	return mockVote{validator: validator}
}

func vote(validator, candidate string, auth bool) mockVote {
	return mockVote{
		validator: validator,
		candidate: candidate,
		auth:      auth,
	}
}

type mockSnapshot struct {
	validators []string
	votes      []mockVote
}

type mockHeader struct {
	action   mockVote
	snapshot *mockSnapshot
}

func newMockHeader(validators []string, vote mockVote) mockHeader {
	return mockHeader{
		action: vote,
		snapshot: &mockSnapshot{
			validators: validators,
			votes:      []mockVote{},
		},
	}
}

func buildHeaders(pool *testerAccountPool, genesis *chain.Genesis, mockHeaders []mockHeader) []*types.Header {
	headers := make([]*types.Header, 0, len(mockHeaders))
	parentHash := genesis.Hash()

	for num, header := range mockHeaders {
		v := header.action
		pool.add(v.validator)

		h := &types.Header{
			Number:     uint64(num + 1),
			ParentHash: parentHash,
			Miner:      types.ZeroAddress,
			MixHash:    IstanbulDigest,
			ExtraData:  genesis.ExtraData,
		}

		if v.candidate != "" {
			// if candidate is empty, we are just creating a new block
			// without votes
			pool.add(v.candidate)
			h.Miner = pool.get(v.candidate).Address()
		}

		if v.auth {
			// add auth to the vote
			h.Nonce = nonceAuthVote
		} else {
			h.Nonce = nonceDropVote
		}

		// sign the vote
		h = pool.get(v.validator).sign(h)
		h.ComputeHash()

		parentHash = h.Hash
		headers = append(headers, h)
	}

	return headers
}

func updateHashesInSnapshots(t *testing.T, b *blockchain.Blockchain, snapshots []*Snapshot) {
	t.Helper()

	for _, s := range snapshots {
		hash := b.GetHashByNumber(s.Number)
		assert.NotNil(t, hash)
		s.Hash = hash.String()
	}
}

func saveSnapshots(t *testing.T, path string, snapshots []*Snapshot) {
	t.Helper()

	if snapshots == nil {
		return
	}

	store := newSnapshotStore()
	for _, snap := range snapshots {
		store.add(snap)
	}

	err := store.saveToPath(path)

	assert.NoError(t, err)
}

func TestSnapshot_setupSnapshot(t *testing.T) {
	// Current validators
	validators := []string{"A", "B", "C", "D"}
	// New voted validators
	candidateValidators := []string{"E", "F"}

	pool := newTesterAccountPool()
	pool.add(validators...)
	validatorSet := pool.ValidatorSet()
	genesis := pool.genesis()

	pool.add(candidateValidators...)

	newSnapshot := func(n uint64, set ValidatorSet, votes []*Vote) *Snapshot {
		return &Snapshot{
			Number: n,
			Set:    set,
			Votes:  votes,
		}
	}

	type snapshotData struct {
		LastBlock uint64
		Snapshots []*Snapshot
	}

	var cases = []struct {
		name           string
		epochSize      uint64
		headers        []mockHeader
		savedSnapshots []*Snapshot
		expectedResult snapshotData
	}{
		{
			name:    "should create genesis",
			headers: []mockHeader{},
			expectedResult: snapshotData{
				LastBlock: 0,
				Snapshots: []*Snapshot{
					newSnapshot(0, validatorSet, []*Vote{}),
				},
			},
		},
		{
			name: "should load from file and advance to latest height without any update if they are in same epoch",
			headers: []mockHeader{
				newMockHeader(validators, skipVote("A")),
				newMockHeader(validators, skipVote("B")),
			},
			savedSnapshots: []*Snapshot{
				newSnapshot(0, validatorSet, []*Vote{}),
			},
			expectedResult: snapshotData{
				LastBlock: 2,
				Snapshots: []*Snapshot{
					newSnapshot(0, validatorSet, []*Vote{}),
				},
			},
		},
		{
			name: "should generate snapshot from genesis because of no snapshot file",
			headers: []mockHeader{
				newMockHeader(validators, skipVote("A")),
				newMockHeader(validators, skipVote("B")),
			},
			savedSnapshots: nil,
			expectedResult: snapshotData{
				LastBlock: 2,
				Snapshots: []*Snapshot{
					newSnapshot(0, validatorSet, []*Vote{}),
				},
			},
		},
		{
			name:      "should generate snapshot from beginning of current epoch because of no snapshot file",
			epochSize: 3,
			headers: []mockHeader{
				newMockHeader(validators, skipVote("A")),
				newMockHeader(validators, skipVote("B")),
				newMockHeader(validators, skipVote("C")),
				newMockHeader(validators, skipVote("D")),
			},
			savedSnapshots: nil,
			expectedResult: snapshotData{
				LastBlock: 4,
				Snapshots: []*Snapshot{
					newSnapshot(3, validatorSet, []*Vote{}),
				},
			},
		},
		{
			name:      "should recover votes from the beginning of current epoch",
			epochSize: 3,
			headers: []mockHeader{
				newMockHeader(validators, skipVote("A")),
				newMockHeader(validators, vote("B", "F", true)),
				newMockHeader(validators, skipVote("C")),
				newMockHeader(validators, vote("D", "E", true)),
			},
			savedSnapshots: nil,
			expectedResult: snapshotData{
				LastBlock: 4,
				Snapshots: []*Snapshot{
					newSnapshot(3, validatorSet, []*Vote{}),
					newSnapshot(4, validatorSet, []*Vote{{
						Validator: pool.get("D").Address(),
						Address:   pool.get("E").Address(),
						Authorize: true,
					}}),
				},
			},
		},
		{
			name: "should not count votes from the beginning of current epoch as there cannot be any proposals " +
				"during the checkpoint block",
			epochSize: 3,
			headers: []mockHeader{
				newMockHeader(validators, skipVote("A")),
				newMockHeader(validators, vote("B", "F", true)),
				newMockHeader(validators, skipVote("C")),
				newMockHeader(validators, vote("D", "E", true)),
				newMockHeader(validators, skipVote("D")),
				newMockHeader(validators, vote("C", "F", true)),
				newMockHeader(validators, vote("A", "F", true)),
			},
			savedSnapshots: nil,
			expectedResult: snapshotData{
				LastBlock: 7,
				Snapshots: []*Snapshot{
					newSnapshot(6, validatorSet, []*Vote{}),
					newSnapshot(7, validatorSet, []*Vote{{
						Validator: pool.get("A").Address(),
						Address:   pool.get("F").Address(),
						Authorize: true,
					}}),
				},
			},
		},
	}

	for _, c := range cases {
		epochSize := c.epochSize
		if epochSize == 0 {
			epochSize = 10
		}

		t.Run(c.name, func(t *testing.T) {
			tmpDir := getTempDir(t)
			// Build blockchain with headers
			blockchain := blockchain.TestBlockchain(t, genesis)
			initialHeaders := buildHeaders(pool, genesis, c.headers)
			for _, h := range initialHeaders {
				err := blockchain.WriteHeaders([]*types.Header{h})
				assert.NoError(t, err)
			}

			ibft := &Ibft{
				epochSize:  epochSize,
				blockchain: blockchain,
				config: &consensus.Config{
					Path: tmpDir,
				},
				logger: hclog.NewNullLogger(),
			}

			initIbftMechanism(PoA, ibft)

			// Write Hash to snapshots
			updateHashesInSnapshots(t, blockchain, c.savedSnapshots)
			updateHashesInSnapshots(t, blockchain, c.expectedResult.Snapshots)
			saveSnapshots(t, tmpDir, c.savedSnapshots)

			assert.NoError(t, ibft.setupSnapshot())
			assert.Equal(t, c.expectedResult.LastBlock, ibft.store.getLastBlock())
			assert.Equal(t, c.expectedResult.Snapshots, ([]*Snapshot)(ibft.store.list))
		})
	}
}

func TestSnapshot_ProcessHeaders(t *testing.T) {
	var cases = []struct {
		name       string
		epochSize  uint64
		validators []string
		headers    []mockHeader
	}{
		{
			name: "single validator casts no vote",
			validators: []string{
				"A",
			},
			headers: []mockHeader{
				{
					action: skipVote("A"),
					snapshot: &mockSnapshot{
						validators: []string{"A"},
					},
				},
			},
		},
		{
			name:       "single validator votes to add two peers",
			validators: []string{"A"},
			headers: []mockHeader{
				{
					// one vote from A is enough to promote B.
					// the vote is not even shown on the result
					action: vote("A", "B", true),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B"},
					},
				},
				{
					action: skipVote("B"),
				},
				{
					// one vote from A is NOT enough to promote C
					// since now B is also a validator
					action: vote("A", "C", true),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B"},
						votes: []mockVote{
							vote("A", "C", true),
						},
					},
				},
			},
		},
		{
			name:       "single validator dropping himself",
			validators: []string{"A"},
			headers: []mockHeader{
				{
					action: vote("A", "A", false),
					snapshot: &mockSnapshot{
						validators: []string{},
					},
				},
			},
		},
		{
			name:       "two validators, dropping requires consensus",
			validators: []string{"A", "B"},
			headers: []mockHeader{
				{
					action: vote("A", "B", false),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B"},
						votes: []mockVote{
							vote("A", "B", false),
						},
					},
				},
				{
					action: vote("B", "B", false),
					snapshot: &mockSnapshot{
						validators: []string{"A"},
					},
				},
			},
		},
		{
			name:       "adding votes are only counted once per validator and target",
			validators: []string{"A", "B"},
			headers: []mockHeader{
				{
					action: vote("A", "C", true),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B"},
						votes: []mockVote{
							vote("A", "C", true),
						},
					},
				},
				{
					action: vote("A", "C", true),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B"},
						votes: []mockVote{
							vote("A", "C", true),
						},
					},
				},
			},
		},
		{
			name:       "delete votes are only counted once per validator and target",
			validators: []string{"A", "B", "C"},
			headers: []mockHeader{
				{
					action: vote("A", "C", false),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes: []mockVote{
							vote("A", "C", false),
						},
					},
				},
				{
					action: vote("A", "C", true),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes: []mockVote{
							vote("A", "C", false),
						},
					},
				},
			},
		},
		{
			name:       "multiple (add, delete) votes are possible",
			validators: []string{"A", "B", "C"},
			headers: []mockHeader{
				{
					action: vote("A", "D", true),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes: []mockVote{
							vote("A", "D", true),
						},
					},
				},
				{
					action: vote("A", "E", true),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes: []mockVote{
							vote("A", "D", true),
							vote("A", "E", true),
						},
					},
				},
				{
					action: vote("A", "B", false),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes: []mockVote{
							vote("A", "D", true),
							vote("A", "E", true),
							vote("A", "B", false),
						},
					},
				},
			},
		},
		{
			name:       "votes from deauthorized nodes are discarded immediately",
			validators: []string{"A", "B", "C"},
			headers: []mockHeader{
				// validator C makes two votes (add and delete)
				{
					action: vote("C", "D", true),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes: []mockVote{
							vote("C", "D", true),
						},
					},
				},
				{
					action: vote("C", "B", false),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes: []mockVote{
							vote("C", "D", true),
							vote("C", "B", false),
						},
					},
				},
				// A and B remove C
				{
					action: vote("A", "C", false),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes: []mockVote{
							vote("C", "D", true),
							vote("C", "B", false),
							vote("A", "C", false),
						},
					},
				},
				// B vote is enough to discard C and clean all the votes
				{
					action: vote("B", "C", false),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B"},
						votes:      []mockVote{},
					},
				},
			},
		},
		{
			name:       "epoch transition resets all votes",
			epochSize:  3,
			validators: []string{"A", "B", "C"},
			headers: []mockHeader{
				{
					// block 1
					action: vote("A", "D", true),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes: []mockVote{
							vote("A", "D", true),
						},
					},
				},
				{
					// block 2
					action: vote("B", "C", false),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes: []mockVote{
							vote("A", "D", true),
							vote("B", "C", false),
						},
					},
				},
				{
					// block 3 (do not vote)
					action: skipVote("B"),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes:      []mockVote{},
					},
				},
			},
		},
		{
			name:       "epoch transition creates new snapshot",
			epochSize:  1,
			validators: []string{"A", "B", "C"},
			headers: []mockHeader{
				{
					// block 1
					action: skipVote("A"),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes:      []mockVote{},
					},
				},
				{
					// block 2
					action: skipVote("B"),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes:      []mockVote{},
					},
				},
				{
					// block 3
					action: skipVote("C"),
					snapshot: &mockSnapshot{
						validators: []string{"A", "B", "C"},
						votes:      []mockVote{},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			epochSize := c.epochSize
			if epochSize == 0 {
				epochSize = 1000
			}

			pool := newTesterAccountPool()
			pool.add(c.validators...)
			genesis := pool.genesis()

			// create votes
			headers := buildHeaders(pool, genesis, c.headers)

			// process the headers independently
			ibft := &Ibft{
				epochSize:  epochSize,
				blockchain: blockchain.TestBlockchain(t, genesis),
				config:     &consensus.Config{},
			}
			initIbftMechanism(PoA, ibft)

			assert.NoError(t, ibft.setupSnapshot())
			for indx, header := range headers {
				if err := ibft.processHeaders([]*types.Header{header}); err != nil {
					t.Fatal(err)
				}

				// get latest snapshot
				snap, err := ibft.getSnapshot(header.Number)
				assert.NoError(t, err)
				assert.NotNil(t, snap)

				result := c.headers[indx].snapshot
				if result != nil {
					resSnap := &Snapshot{
						Votes: []*Vote{},
						Set:   ValidatorSet{},
					}
					// check validators
					for _, i := range result.validators {
						resSnap.Set.Add(pool.get(i).Address())
					}
					// build result votes
					for _, v := range result.votes {
						resSnap.Votes = append(resSnap.Votes, &Vote{
							Validator: pool.get(v.validator).Address(),
							Address:   pool.get(v.candidate).Address(),
							Authorize: v.auth,
						})
					}
					if !resSnap.Equal(snap) {
						t.Fatal("bad")
					}
				}
			}

			// check the metadata
			meta, err := ibft.getSnapshotMetadata()
			assert.NoError(t, err)

			if meta.LastBlock != headers[len(headers)-1].Number {
				t.Fatal("incorrect meta")
			}

			// Process headers all at the same time should have the same result
			ibft1 := &Ibft{
				epochSize:  epochSize,
				blockchain: blockchain.TestBlockchain(t, genesis),
				config:     &consensus.Config{},
			}

			initIbftMechanism(PoA, ibft1)

			assert.NoError(t, ibft1.setupSnapshot())
			if err := ibft1.processHeaders(headers); err != nil {
				t.Fatal(err)
			}

			// from 0 to last header check that all the snapshots match
			for i := uint64(0); i < headers[len(headers)-1].Number; i++ {
				snap0, err := ibft.getSnapshot(i)
				assert.NoError(t, err)

				snap1, err := ibft1.getSnapshot(i)
				assert.NoError(t, err)

				if !snap0.Equal(snap1) {
					t.Fatal("bad")
				}
			}
		})
	}
}

func TestSnapshot_PurgeSnapshots(t *testing.T) {
	pool := newTesterAccountPool()
	pool.add("a", "b", "c")

	genesis := pool.genesis()
	ibft1 := &Ibft{
		epochSize:  10,
		blockchain: blockchain.TestBlockchain(t, genesis),
		config:     &consensus.Config{},
	}
	assert.NoError(t, ibft1.setupSnapshot())
	initIbftMechanism(PoA, ibft1)

	// write a header that creates a snapshot
	headers := []*types.Header{}

	for i := 1; i < 51; i++ {
		id := strconv.Itoa(i)
		pool.add(id)

		h := &types.Header{
			Number:     uint64(i),
			ParentHash: ibft1.blockchain.Header().Hash,
			Miner:      types.ZeroAddress,
			MixHash:    IstanbulDigest,
			ExtraData:  genesis.ExtraData,
		}

		h.Miner = pool.get(id).Address()
		h.Nonce = nonceAuthVote

		h = pool.get("a").sign(h)
		h.ComputeHash()
		headers = append(headers, h)
	}

	err := ibft1.processHeaders(headers)
	assert.NoError(t, err)

	assert.Equal(t, len(ibft1.store.list), 21)
}

func TestSnapshot_Store_SaveLoad(t *testing.T) {
	tmpDir := getTempDir(t)
	store0 := newSnapshotStore()

	for i := 0; i < 10; i++ {
		store0.add(&Snapshot{
			Number: uint64(i),
		})
	}
	assert.NoError(t, store0.saveToPath(tmpDir))

	store1 := newSnapshotStore()
	assert.NoError(t, store1.loadFromPath(tmpDir, hclog.NewNullLogger()))

	assert.Equal(t, store0, store1)
}

func TestSnapshot_Store_Find(t *testing.T) {
	store := newSnapshotStore()

	for i := 0; i <= 100; i++ {
		if i%10 == 0 {
			store.add(&Snapshot{
				Number: uint64(i),
			})
		}
	}

	check := func(num, expected uint64) {
		assert.Equal(t, store.find(num).Number, expected)
	}

	check(0, 0)
	check(19, 10)
	check(20, 20)
	check(21, 20)
	check(1000, 100)
}
