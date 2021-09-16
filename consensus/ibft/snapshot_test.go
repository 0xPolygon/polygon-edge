package ibft

import (
	"crypto/ecdsa"
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/consensus"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func getTempDir(t *testing.T) string {
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

type mockSnapshot struct {
	validators []string
}

type mockHeader struct {
	signer   string
	snapshot *mockSnapshot
}

func newMockHeader(validators []string, signer string) mockHeader {
	return mockHeader{
		signer: signer,
		snapshot: &mockSnapshot{
			validators: validators,
		},
	}
}

func buildHeaders(pool *testerAccountPool, genesis *chain.Genesis, mockHeaders []mockHeader) []*types.Header {
	headers := make([]*types.Header, 0, len(mockHeaders))
	parentHash := genesis.Hash()
	for num, header := range mockHeaders {
		//v := header.action
		//pool.add(v.validator)
		signer := header.signer

		h := &types.Header{
			Number:     uint64(num + 1),
			ParentHash: parentHash,
			Miner:      types.ZeroAddress,
			MixHash:    IstanbulDigest,
			ExtraData:  genesis.ExtraData,
		}

		// sign the header
		h = pool.get(signer).sign(h)
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

	pool := newTesterAccountPool()
	pool.add(validators...)
	validatorSet := pool.ValidatorSet()
	genesis := pool.genesis()

	newSnapshot := func(n uint64, set ValidatorSet) *Snapshot {
		return &Snapshot{
			Number: n,
			Set:    set,
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
					newSnapshot(0, validatorSet),
				},
			},
		},
		{
			name: "should load from file and advance to latest height without any update if they are in same epoch",
			headers: []mockHeader{
				newMockHeader(validators, "A"),
				newMockHeader(validators, "B"),
			},
			savedSnapshots: []*Snapshot{
				newSnapshot(0, validatorSet),
			},
			expectedResult: snapshotData{
				LastBlock: 2,
				Snapshots: []*Snapshot{
					newSnapshot(0, validatorSet),
				},
			},
		},
		{
			name: "should generate snapshot from genesis because of no snapshot file",
			headers: []mockHeader{
				newMockHeader(validators, "A"),
				newMockHeader(validators, "B"),
			},
			savedSnapshots: nil,
			expectedResult: snapshotData{
				LastBlock: 2,
				Snapshots: []*Snapshot{
					newSnapshot(0, validatorSet),
				},
			},
		},
		{
			name:      "should generate snapshot from beginning of current epoch because of no snapshot file",
			epochSize: 3,
			headers: []mockHeader{
				newMockHeader(validators, "A"),
				newMockHeader(validators, "B"),
				newMockHeader(validators, "C"),
				newMockHeader(validators, "D"),
			},
			savedSnapshots: nil,
			expectedResult: snapshotData{
				LastBlock: 4,
				Snapshots: []*Snapshot{
					newSnapshot(3, validatorSet),
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

		h = pool.get("a").sign(h)
		h.ComputeHash()
		headers = append(headers, h)
	}

	err := ibft1.processHeaders(headers)
	assert.NoError(t, err)

	assert.Equal(t, 3, len(ibft1.store.list))
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
	assert.NoError(t, store1.loadFromPath(tmpDir))

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
