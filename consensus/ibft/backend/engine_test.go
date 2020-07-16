package backend

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/blockchain/storage/memory"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/state"
	itrie "github.com/0xPolygon/minimal/state/immutable-trie"
	"github.com/0xPolygon/minimal/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// in this test, we can set n to 1, and it means we can process Istanbul and commit a
// block by one node. Otherwise, if n is larger than 1, we have to generate
// other fake events to process Istanbul.
func newBlockChain(n int) (*blockchain.Blockchain, *backend) {
	genesis, nodeKeys := getGenesisAndKeys(n)
	memDB, _ := memory.NewMemoryStorage(nil)
	st := itrie.NewState(itrie.NewMemoryStorage())
	config := ibft.DefaultConfig
	// Use the first key as private key
	b, _ := New(config, nodeKeys[0], memDB).(*backend)

	chainConfig := &chain.Params{
		Forks: &chain.Forks{
			Homestead:      chain.NewFork(0),
			EIP150:         chain.NewFork(0),
			EIP155:         chain.NewFork(0),
			EIP158:         chain.NewFork(0),
			Byzantium:      chain.NewFork(0),
			Constantinople: chain.NewFork(0),
			Petersburg:     chain.NewFork(0),
		},
	}

	executor := state.NewExecutor(chainConfig, st)
	executor.WriteGenesis(genesis.Alloc)
	blockchain := blockchain.NewBlockchain(memDB, genesis.Config, b, executor)
	b.Start(blockchain, blockchain.CurrentBlock, blockchain.HasBadBlock)
	snap, err := b.snapshot(blockchain, 0, types.Hash{}, nil)
	if err != nil {
		panic(err)
	}
	if snap == nil {
		panic("failed to get snapshot")
	}
	proposerAddr := snap.ValSet.GetProposer().Address()

	// find proposer key
	for _, key := range nodeKeys {
		addr := crypto.PubKeyToAddress(&key.PublicKey)
		if addr.String() == proposerAddr.String() {
			b.privateKey = key
			b.address = addr
		}
	}

	return blockchain, b
}

func getGenesisAndKeys(n int) (*chain.Genesis, []*ecdsa.PrivateKey) {
	// Setup validators
	var nodeKeys = make([]*ecdsa.PrivateKey, n)
	var addrs = make([]types.Address, n)
	for i := 0; i < n; i++ {
		nodeKeys[i], _ = crypto.GenerateKey()
		addrs[i] = crypto.PubKeyToAddress(&nodeKeys[i].PublicKey)
	}

	// generate genesis block
	genesis := &chain.Genesis{
		Difficulty: 18,
		GasLimit:   17,
	}
	chainConfig := &chain.Params{
		Forks: &chain.Forks{
			Homestead:      chain.NewFork(0),
			EIP150:         chain.NewFork(0),
			EIP155:         chain.NewFork(0),
			EIP158:         chain.NewFork(0),
			Byzantium:      chain.NewFork(0),
			Constantinople: chain.NewFork(0),
			Petersburg:     chain.NewFork(0),
		},
	}
	genesis.Config = chainConfig
	// force enable Istanbul engine
	genesis.Difficulty = defaultDifficulty.Uint64()
	genesis.Nonce = emptyNonce
	genesis.Mixhash = types.IstanbulDigest

	appendValidators(genesis, addrs)
	return genesis, nodeKeys
}

func appendValidators(genesis *chain.Genesis, addrs []types.Address) {

	if len(genesis.ExtraData) < types.IstanbulExtraVanity {
		genesis.ExtraData = append(genesis.ExtraData, bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity)...)
	}
	genesis.ExtraData = genesis.ExtraData[:types.IstanbulExtraVanity]

	ist := &types.IstanbulExtra{
		Validators:    addrs,
		Seal:          []byte{},
		CommittedSeal: [][]byte{},
	}

	istPayload, err := rlp.EncodeToBytes(&ist)
	if err != nil {
		panic("failed to encode istanbul extra")
	}
	genesis.ExtraData = append(genesis.ExtraData, istPayload...)
}

func makeHeader(parent *types.Block, config *ibft.Config) *types.Header {
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     new(big.Int).SetUint64(parent.Header.Number).Add(new(big.Int).SetUint64(parent.Header.Number), types.Big1).Uint64(),
		GasLimit:   parent.Header.GasLimit,
		GasUsed:    0,
		ExtraData:  parent.Header.ExtraData,
		Timestamp:  new(big.Int).Add(new(big.Int).SetUint64(parent.Header.Timestamp), new(big.Int).SetUint64(config.BlockPeriod)).Uint64(),
		Difficulty: defaultDifficulty.Uint64(),
	}
	return header
}

func makeBlock(chain *blockchain.Blockchain, engine *backend, parent *types.Block) *types.Block {
	block := makeBlockWithoutSeal(chain, engine, parent)
	block, _ = engine.Seal(chain, block, context.Background())
	return block
}

func makeBlockWithoutSeal(chain *blockchain.Blockchain, engine *backend, parent *types.Block) *types.Block {
	header := makeHeader(parent, engine.config)
	engine.Prepare(chain, header)

	state, _ := chain.Executor().StateAt(parent.Header.StateRoot)
	block, _ := engine.Finalize(chain, header, state, nil, nil, nil)
	return block
}

func TestPrepare(t *testing.T) {
	chain, engine := newBlockChain(1)
	block, _ := chain.GetBlockByHash(chain.Genesis(), true)
	header := makeHeader(block, engine.config)
	err := engine.Prepare(chain, header)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	header.ParentHash = types.StringToHash("1234567890")
	err = engine.Prepare(chain, header)
	if err != consensus.ErrUnknownAncestor {
		t.Errorf("error mismatch: have %v, want %v", err, consensus.ErrUnknownAncestor)
	}
}

func TestSealStopChannel(t *testing.T) {
	chain, engine := newBlockChain(4)
	genesisBlock, _ := chain.GetBlockByHash(chain.Genesis(), true)
	block := makeBlockWithoutSeal(chain, engine, genesisBlock)
	ctx := context.Background()
	eventSub := engine.EventMux().Subscribe(ibft.RequestEvent{})
	eventLoop := func() {
		ev := <-eventSub.Chan()
		_, ok := ev.Data.(ibft.RequestEvent)
		if !ok {
			t.Errorf("unexpected event comes: %v", reflect.TypeOf(ev.Data))
		}
		ctx.Done()
		eventSub.Unsubscribe()
	}
	go eventLoop()
	finalBlock, err := engine.Seal(chain, block, ctx)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if finalBlock != nil {
		t.Errorf("block mismatch: have %v, want nil", finalBlock)
	}
}

func TestSealCommittedOtherHash(t *testing.T) {
	chain, engine := newBlockChain(4)
	genesis, _ := chain.GetBlockByHash(chain.Genesis(), true)
	block := makeBlockWithoutSeal(chain, engine, genesis)
	otherBlock := makeBlockWithoutSeal(chain, engine, block)
	eventSub := engine.EventMux().Subscribe(ibft.RequestEvent{})
	eventLoop := func() {
		ev := <-eventSub.Chan()
		_, ok := ev.Data.(ibft.RequestEvent)
		if !ok {
			t.Errorf("unexpected event comes: %v", reflect.TypeOf(ev.Data))
		}
		engine.Commit(otherBlock, [][]byte{})
		eventSub.Unsubscribe()
	}
	go eventLoop()
	seal := func() {
		engine.Seal(chain, block, nil)
		t.Error("seal should not be completed")
	}
	go seal()

	const timeoutDura = 2 * time.Second
	timeout := time.NewTimer(timeoutDura)
	<-timeout.C
	// wait 2 seconds to ensure we cannot get any blocks from Istanbul
}

func TestSealCommitted(t *testing.T) {
	chain, engine := newBlockChain(1)
	genesis, _ := chain.GetBlockByHash(chain.Genesis(), true)
	block := makeBlockWithoutSeal(chain, engine, genesis)
	header, _ := engine.chain.GetHeader(block.ParentHash(), block.Number()-1)
	expectedBlock, _ := engine.updateBlock(header, block)

	finalBlock, err := engine.Seal(chain, block, nil)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if finalBlock.Hash() != expectedBlock.Hash() {
		t.Errorf("hash mismatch: have %v, want %v", finalBlock.Hash(), expectedBlock.Hash())
	}
}

func TestVerifyHeader(t *testing.T) {
	chain, engine := newBlockChain(1)

	genesis, _ := chain.GetBlockByHash(chain.Genesis(), true)
	// errEmptyCommittedSeals case
	block := makeBlockWithoutSeal(chain, engine, genesis)
	block, _ = engine.updateBlock(genesis.Header, block)
	err := engine.VerifyHeader(chain, block.Header, false, false)
	if err != errEmptyCommittedSeals {
		t.Errorf("error mismatch: have %v, want %v", err, errEmptyCommittedSeals)
	}

	// short extra data
	header := block.Header
	header.ExtraData = []byte{}
	err = engine.VerifyHeader(chain, header, false, false)
	if err != errInvalidExtraDataFormat {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidExtraDataFormat)
	}
	// incorrect extra format
	header.ExtraData = []byte("0000000000000000000000000000000012300000000000000000000000000000000000000000000000000000000000000000")
	err = engine.VerifyHeader(chain, header, false, false)
	if err != errInvalidExtraDataFormat {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidExtraDataFormat)
	}

	// non zero MixDigest
	block = makeBlockWithoutSeal(chain, engine, genesis)
	header = block.Header
	header.MixHash = types.StringToHash("123456789")
	err = engine.VerifyHeader(chain, header, false, false)
	if err != errInvalidMixDigest {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidMixDigest)
	}

	// invalid uncles hash
	block = makeBlockWithoutSeal(chain, engine, genesis)
	header = block.Header
	header.Sha3Uncles = types.StringToHash("123456789")
	err = engine.VerifyHeader(chain, header, false, false)
	if err != errInvalidUncleHash {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidUncleHash)
	}

	// invalid difficulty
	block = makeBlockWithoutSeal(chain, engine, genesis)
	header = block.Header
	header.Difficulty = 2
	err = engine.VerifyHeader(chain, header, false, false)
	if err != errInvalidDifficulty {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidDifficulty)
	}

	// invalid timestamp
	block = makeBlockWithoutSeal(chain, engine, genesis)
	header = block.Header
	header.Timestamp = new(big.Int).Add(new(big.Int).SetUint64(genesis.Header.Timestamp), new(big.Int).SetUint64(engine.config.BlockPeriod-1)).Uint64()
	err = engine.VerifyHeader(chain, header, false, false)
	if err != errInvalidTimestamp {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidTimestamp)
	}

	// future block
	block = makeBlockWithoutSeal(chain, engine, genesis)
	header = block.Header
	header.Timestamp = new(big.Int).Add(big.NewInt(now().Unix()), new(big.Int).SetUint64(10)).Uint64()
	err = engine.VerifyHeader(chain, header, false, false)
	if err != consensus.ErrFutureBlock {
		t.Errorf("error mismatch: have %v, want %v", err, consensus.ErrFutureBlock)
	}

	// invalid nonce
	block = makeBlockWithoutSeal(chain, engine, genesis)
	header = block.Header
	copy(header.Nonce[:], hex.MustDecodeHex("0x111111111111"))
	header.Number = big.NewInt(int64(engine.config.Epoch)).Uint64()
	err = engine.VerifyHeader(chain, header, false, false)
	if err != errInvalidNonce {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidNonce)
	}
}

func TestVerifySeal(t *testing.T) {
	chain, engine := newBlockChain(1)
	genesis, _ := chain.GetBlockByHash(chain.Genesis(), true)
	// cannot verify genesis
	err := engine.VerifySeal(chain, genesis.Header)
	if err != errUnknownBlock {
		t.Errorf("error mismatch: have %v, want %v", err, errUnknownBlock)
	}

	block := makeBlock(chain, engine, genesis)
	// change block content
	header := block.Header
	header.Number = big.NewInt(4).Uint64()
	block1 := block.WithSeal(header)
	err = engine.VerifySeal(chain, block1.Header)
	if err != errUnauthorized {
		t.Errorf("error mismatch: have %v, want %v", err, errUnauthorized)
	}

	// unauthorized users but still can get correct signer address
	engine.privateKey, _ = crypto.GenerateKey()
	err = engine.VerifySeal(chain, block.Header)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
}

func TestVerifyHeaders(t *testing.T) {
	chain, engine := newBlockChain(1)
	genesis, _ := chain.GetBlockByHash(chain.Genesis(), true)

	// success case
	headers := []*types.Header{}
	blocks := []*types.Block{}
	size := 100

	for i := 0; i < size; i++ {
		var b *types.Block
		if i == 0 {
			b = makeBlockWithoutSeal(chain, engine, genesis)
			b, _ = engine.updateBlock(genesis.Header, b)
		} else {
			b = makeBlockWithoutSeal(chain, engine, blocks[i-1])
			b, _ = engine.updateBlock(blocks[i-1].Header, b)
		}
		blocks = append(blocks, b)
		headers = append(headers, blocks[i].Header)
	}
	now = func() time.Time {
		return time.Unix(int64(headers[size-1].Timestamp), 0)
	}
	_, results := engine.VerifyHeaders(chain, headers, nil)
	const timeoutDura = 2 * time.Second
	timeout := time.NewTimer(timeoutDura)
	index := 0
OUT1:
	for {
		select {
		case err := <-results:
			if err != nil {
				if err != errEmptyCommittedSeals && err != errInvalidCommittedSeals {
					t.Errorf("error mismatch: have %v, want errEmptyCommittedSeals|errInvalidCommittedSeals", err)
					break OUT1
				}
			}
			index++
			if index == size {
				break OUT1
			}
		case <-timeout.C:
			break OUT1
		}
	}
	// abort cases
	abort, results := engine.VerifyHeaders(chain, headers, nil)
	timeout = time.NewTimer(timeoutDura)
	index = 0
OUT2:
	for {
		select {
		case err := <-results:
			if err != nil {
				if err != errEmptyCommittedSeals && err != errInvalidCommittedSeals {
					t.Errorf("error mismatch: have %v, want errEmptyCommittedSeals|errInvalidCommittedSeals", err)
					break OUT2
				}
			}
			index++
			if index == 5 {
				abort <- struct{}{}
			}
			if index >= size {
				t.Errorf("verifyheaders should be aborted")
				break OUT2
			}
		case <-timeout.C:
			break OUT2
		}
	}
	// error header cases
	headers[2].Number = big.NewInt(100).Uint64()
	abort, results = engine.VerifyHeaders(chain, headers, nil)
	timeout = time.NewTimer(timeoutDura)
	index = 0
	errors := 0
	expectedErrors := 2
OUT3:
	for {
		select {
		case err := <-results:
			if err != nil {
				if err != errEmptyCommittedSeals && err != errInvalidCommittedSeals {
					errors++
				}
			}
			index++
			if index == size {
				if errors != expectedErrors {
					t.Errorf("error mismatch: have %v, want %v", err, expectedErrors)
				}
				break OUT3
			}
		case <-timeout.C:
			break OUT3
		}
	}
}

func TestPrepareExtra(t *testing.T) {
	validators := make([]types.Address, 4)
	validators[0] = types.BytesToAddress(hex.MustDecodeHex("0x44add0ec310f115a0e603b2d7db9f067778eaf8a"))
	validators[1] = types.BytesToAddress(hex.MustDecodeHex("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212"))
	validators[2] = types.BytesToAddress(hex.MustDecodeHex("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6"))
	validators[3] = types.BytesToAddress(hex.MustDecodeHex("0x8be76812f765c24641ec63dc2852b378aba2b440"))

	vanity := make([]byte, types.IstanbulExtraVanity)
	expectedResult := append(vanity, hex.MustDecodeHex("0xf858f8549444add0ec310f115a0e603b2d7db9f067778eaf8a94294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212946beaaed781d2d2ab6350f5c4566a2c6eaac407a6948be76812f765c24641ec63dc2852b378aba2b44080c0")...)

	h := &types.Header{
		ExtraData: vanity,
	}

	payload, err := prepareExtra(h, validators)
	if err != nil {
		t.Errorf("error mismatch: have %v, want: nil", err)
	}
	if !reflect.DeepEqual(payload, expectedResult) {
		t.Errorf("payload mismatch: have %v, want %v", payload, expectedResult)
	}

	// append useless information to extra-data
	h.ExtraData = append(vanity, make([]byte, 15)...)

	payload, err = prepareExtra(h, validators)
	if !reflect.DeepEqual(payload, expectedResult) {
		t.Errorf("payload mismatch: have %v, want %v", payload, expectedResult)
	}
}

func TestWriteSeal(t *testing.T) {
	vanity := bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity)
	istRawData := hex.MustDecodeHex("0xf858f8549444add0ec310f115a0e603b2d7db9f067778eaf8a94294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212946beaaed781d2d2ab6350f5c4566a2c6eaac407a6948be76812f765c24641ec63dc2852b378aba2b44080c0")
	expectedSeal := append([]byte{1, 2, 3}, bytes.Repeat([]byte{0x00}, types.IstanbulExtraSeal-3)...)
	expectedIstExtra := &types.IstanbulExtra{
		Validators: []types.Address{
			types.BytesToAddress(hex.MustDecodeHex("0x44add0ec310f115a0e603b2d7db9f067778eaf8a")),
			types.BytesToAddress(hex.MustDecodeHex("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212")),
			types.BytesToAddress(hex.MustDecodeHex("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
			types.BytesToAddress(hex.MustDecodeHex("0x8be76812f765c24641ec63dc2852b378aba2b440")),
		},
		Seal:          expectedSeal,
		CommittedSeal: [][]byte{},
	}
	var expectedErr error

	h := &types.Header{
		ExtraData: append(vanity, istRawData...),
	}

	// normal case
	err := writeSeal(h, expectedSeal)
	if err != expectedErr {
		t.Errorf("error mismatch: have %v, want %v", err, expectedErr)
	}

	// verify istanbul extra-data
	istExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if !reflect.DeepEqual(istExtra, expectedIstExtra) {
		t.Errorf("extra data mismatch: have %v, want %v", istExtra, expectedIstExtra)
	}

	// invalid seal
	unexpectedSeal := append(expectedSeal, make([]byte, 1)...)
	err = writeSeal(h, unexpectedSeal)
	if err != errInvalidSignature {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidSignature)
	}
}

func TestWriteCommittedSeals(t *testing.T) {
	vanity := bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity)
	istRawData := hex.MustDecodeHex("0xf858f8549444add0ec310f115a0e603b2d7db9f067778eaf8a94294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212946beaaed781d2d2ab6350f5c4566a2c6eaac407a6948be76812f765c24641ec63dc2852b378aba2b44080c0")
	expectedCommittedSeal := append([]byte{1, 2, 3}, bytes.Repeat([]byte{0x00}, types.IstanbulExtraSeal-3)...)
	expectedIstExtra := &types.IstanbulExtra{
		Validators: []types.Address{
			types.BytesToAddress(hex.MustDecodeHex("0x44add0ec310f115a0e603b2d7db9f067778eaf8a")),
			types.BytesToAddress(hex.MustDecodeHex("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212")),
			types.BytesToAddress(hex.MustDecodeHex("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
			types.BytesToAddress(hex.MustDecodeHex("0x8be76812f765c24641ec63dc2852b378aba2b440")),
		},
		Seal:          []byte{},
		CommittedSeal: [][]byte{expectedCommittedSeal},
	}
	var expectedErr error

	h := &types.Header{
		ExtraData: append(vanity, istRawData...),
	}

	// normal case
	err := writeCommittedSeals(h, [][]byte{expectedCommittedSeal})
	if err != expectedErr {
		t.Errorf("error mismatch: have %v, want %v", err, expectedErr)
	}

	// verify istanbul extra-data
	istExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if !reflect.DeepEqual(istExtra, expectedIstExtra) {
		t.Errorf("extra data mismatch: have %v, want %v", istExtra, expectedIstExtra)
	}

	// invalid seal
	unexpectedCommittedSeal := append(expectedCommittedSeal, make([]byte, 1)...)
	err = writeCommittedSeals(h, [][]byte{unexpectedCommittedSeal})
	if err != errInvalidCommittedSeals {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidCommittedSeals)
	}
}
