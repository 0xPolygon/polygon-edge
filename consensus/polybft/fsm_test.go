package polybft

// import (
// 	"math/rand"
// 	"testing"

// 	pbft "github.com/0xPolygon/pbft-consensus"
// 	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
// 	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
// 	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
// 	"github.com/0xPolygon/polygon-edge/types"
// 	"github.com/ethereum/go-ethereum/common"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/mock"
// 	"github.com/stretchr/testify/require"
// )

// func TestFSM_ValidateHeader(t *testing.T) {
// 	parent := &types.Header{
// 		Number: 0,
// 	}
// 	parent.ComputeHash()

// 	header := &types.Header{
// 		Number: 0,
// 	}

// 	// parent hash
// 	assert.ErrorContains(t, validateHeaderFields(parent, header), "incorrect header parent hash")
// 	header.ParentHash = parent.Hash

// 	// sequence number
// 	assert.ErrorContains(t, validateHeaderFields(parent, header), "invalid number")
// 	header.Number = 1

// 	// failed timestamp
// 	assert.ErrorContains(t, validateHeaderFields(parent, header), "timestamp older than parent")
// 	header.Timestamp = 10

// 	// mixdigest
// 	assert.ErrorContains(t, validateHeaderFields(parent, header), "mix digest is not correct")
// 	header.MixHash = PolyMixDigest

// 	// difficulty
// 	header.Difficulty = 0
// 	assert.ErrorContains(t, validateHeaderFields(parent, header), "difficulty should be greater than zero")

// 	header.Difficulty = 1
// 	assert.NoError(t, validateHeaderFields(parent, header))
// }

// // func TestFSM_verifyValidatorsUptimeTx(t *testing.T) {
// // 	fsm := &fsm{
// // 		config:        &PolyBFTConfig{ValidatorSetAddr: contracts.ValidatorSetContract},
// // 		isEndOfEpoch:  true,
// // 		uptimeCounter: createTestUptimeCounter(t, nil, 10),
// // 	}

// // 	// include uptime transaction to the epoch ending block
// // 	uptimeTx, err := fsm.createValidatorsUptimeTx()
// // 	assert.NoError(t, err)
// // 	assert.NotNil(t, uptimeTx)
// // 	transactions := []*types.Transaction{uptimeTx}
// // 	block := types.NewBlock(
// // 		&types.Header{GasLimit: stateTransactionsGasLimit},
// // 		transactions,
// // 		nil,
// // 		nil,
// // 		trie.NewStackTrie(nil),
// // 	)
// // 	assert.NoError(t, fsm.verifyValidatorsUptimeTx(block.Transactions()))

// // 	// don't include validators uptime transaction to the epoch ending block
// // 	block = types.NewBlock(&types.Header{GasLimit: stateTransactionsGasLimit}, nil, nil, nil, trie.NewStackTrie(nil))
// // 	assert.Error(t, fsm.verifyValidatorsUptimeTx(block.Transactions()))

// // 	// submit tampered validators uptime transaction to the epoch ending block
// // 	alteredUptimeTx := types.NewTx(&types.StateTransaction{
// // 		To:    fsm.config.ValidatorSetAddr,
// // 		Input: []byte{},
// // 		Gas:   0,
// // 	})
// // 	transactions = []*types.Transaction{alteredUptimeTx}
// // 	block = types.NewBlock(
// // 		&types.Header{GasLimit: stateTransactionsGasLimit},
// // 		transactions,
// // 		nil,
// // 		nil,
// // 		trie.NewStackTrie(nil),
// // 	)
// // 	assert.Error(t, fsm.verifyValidatorsUptimeTx(block.Transactions()))

// // 	fsm.isEndOfEpoch = false
// // 	// submit validators uptime transaction to the non-epoch ending block
// // 	uptimeTx, err = fsm.createValidatorsUptimeTx()
// // 	assert.NoError(t, err)
// // 	assert.NotNil(t, uptimeTx)
// // 	transactions = []*types.Transaction{uptimeTx}
// // 	block = types.NewBlock(
// // 		&types.Header{GasLimit: stateTransactionsGasLimit},
// // 		transactions,
// // 		nil,
// // 		nil,
// // 		trie.NewStackTrie(nil),
// // 	)
// // 	assert.Error(t, fsm.verifyValidatorsUptimeTx(block.Transactions()))

// // 	// create block with dummy transaction in non-epoch ending block
// // 	dummyEip1559Tx := types.NewTx(&types.DynamicFeeTx{
// // 		Nonce:     1,
// // 		Gas:       1000000,
// // 		To:        &common.Address{},
// // 		Value:     big.NewInt(1),
// // 		GasTipCap: big.NewInt(500),
// // 		GasFeeCap: big.NewInt(500),
// // 	})
// // 	transactions = []*types.Transaction{dummyEip1559Tx}
// // 	block = types.NewBlock(
// // 		&types.Header{GasLimit: stateTransactionsGasLimit},
// // 		transactions,
// // 		nil,
// // 		nil,
// // 		trie.NewStackTrie(nil),
// // 	)
// // 	assert.NoError(t, fsm.verifyValidatorsUptimeTx(block.Transactions()))
// // }

// func TestFSM_Init(t *testing.T) {
// 	mblockBuilder := &blockBuilderMock{}
// 	mblockBuilder.On("Reset").Once()
// 	fsm := &fsm{blockBuilder: mblockBuilder}
// 	fsm.Init(&pbft.RoundInfo{})
// 	assert.Nil(t, fsm.proposal)
// 	mblockBuilder.AssertExpectations(t)
// }

// func TestFSM_BuildProposal_WithoutUptimeTxGood(t *testing.T) {

// 	const (
// 		epoch                    = 0
// 		accountCount             = 5
// 		committedCount           = 4
// 		parentCount              = 3
// 		confirmedStateSyncsCount = 5
// 		parentBlockID            = 1023
// 	)
// 	validators := newTestValidators(accountCount)
// 	validatorSet := validators.getPublicIdentities()
// 	extra := createTestExtra(validatorSet, AccountSet{}, accountCount-1, committedCount, parentCount)

// 	parent := &types.Header{Number: parentBlockID, ExtraData: extra}
// 	parent.ComputeHash()
// 	buildBlock := createDummyStateBlock(parentBlockID+1, parent.Hash)
// 	mBlockBuilder := newBlockBuilderMock(buildBlock)

// 	commitment := createTestCommitment(t, validators.getPrivateIdentities())
// 	fsm := &fsm{parent: parent, blockBuilder: mBlockBuilder, config: &PolyBFTConfig{}, backend: &blockchainMock{},
// 		proposerCommitmentToRegister: commitment, validators: newValidatorSet(common.Address{}, validatorSet)}

// 	proposal, err := fsm.BuildProposal()
// 	assert.NoError(t, err)
// 	assert.NotNil(t, proposal)

// 	rlpBlock, err := buildBlock.EncodeRlpBlock()
// 	assert.NoError(t, err)
// 	assert.Equal(t, rlpBlock, proposal.Data)
// 	assert.Equal(t, buildBlock.Block.Hash().Bytes(), proposal.Hash)

// 	mBlockBuilder.AssertExpectations(t)
// }

// func generateValidatorDelta(validatorCount int, allAccounts, previousValidatorSet AccountSet) (vd *ValidatorSetDelta) {
// 	oldMap := make(map[common.Address]int, previousValidatorSet.Len())
// 	for i, x := range previousValidatorSet {
// 		oldMap[x.Address] = i
// 	}

// 	vd = &ValidatorSetDelta{}
// 	vd.Removed = bitmap.Bitmap{}

// 	for _, id := range rand.Perm(len(allAccounts))[:validatorCount] {
// 		_, exists := oldMap[allAccounts[id].Address]
// 		if !exists {
// 			vd.Added = append(vd.Added, allAccounts[id])
// 		}
// 		delete(oldMap, allAccounts[id].Address)
// 	}

// 	for _, v := range oldMap {
// 		vd.Removed.Set(uint64(v))
// 	}

// 	return
// }

// func createDummyStateBlock(blockNumber uint64, parentHash common.Hash) *StateBlock {
// 	header := &types.Header{Number: blockNumber, ParentHash: parentHash, Difficulty: 1}
// 	finalBlock := blockbuilder.NewFinalBlock(header, []*types.Transaction{}, []*types.Receipt{})
// 	return &StateBlock{Block: finalBlock}
// }

// func createTestExtra(
// 	allAccounts,
// 	previousValidatorSet AccountSet,
// 	validatorsCount,
// 	committedSignaturesCount,
// 	parentSignaturesCount int,
// ) []byte {
// 	accountCount := len(allAccounts)
// 	dummySignature := [64]byte{}
// 	bitmapCommitted, bitmapParent := bitmap.Bitmap{}, bitmap.Bitmap{}
// 	extraData := Extra{}
// 	extraData.Validators = generateValidatorDelta(validatorsCount, allAccounts, previousValidatorSet)

// 	for j := range rand.Perm(accountCount)[:committedSignaturesCount] {
// 		bitmapCommitted.Set(uint64(j))
// 	}

// 	for j := range rand.Perm(accountCount)[:parentSignaturesCount] {
// 		bitmapParent.Set(uint64(j))
// 	}

// 	extraData.Parent = &Signature{Bitmap: bitmapCommitted, AggregatedSignature: dummySignature[:]}
// 	extraData.Committed = &Signature{Bitmap: bitmapParent, AggregatedSignature: dummySignature[:]}
// 	marshaled := extraData.MarshalRLPTo(nil)
// 	result := make([]byte, ExtraVanity+len(marshaled))
// 	copy(result[ExtraVanity:], marshaled)
// 	return result
// }

// func createTestCommitment(t *testing.T, accounts []*wallet.Account) *CommitmentMessageSigned {
// 	bitmap := bitmap.Bitmap{}
// 	stateSyncEvents := make([]*StateSyncEvent, len(accounts))
// 	for i := 0; i < len(accounts); i++ {
// 		stateSyncEvents[i] = newStateSyncEvent(
// 			uint64(i),
// 			types.Address(accounts[i].Ecdsa.Address()),
// 			types.Address(accounts[0].Ecdsa.Address()),
// 			[]byte{}, nil,
// 		)
// 		bitmap.Set(uint64(i))
// 	}

// 	stateSyncsTrie, err := createMerkleTree(stateSyncEvents, stateSyncBundleSize)
// 	require.NoError(t, err)

// 	commitment := NewCommitmentMessage(stateSyncsTrie.Hash(), 0, uint64(len(stateSyncEvents)), stateSyncBundleSize)
// 	hash, err := commitment.Hash()
// 	require.NoError(t, err)

// 	var signatures bls.Signatures
// 	for _, a := range accounts {
// 		signature, err := a.Bls.Sign(hash.Bytes())
// 		assert.NoError(t, err)

// 		signatures = append(signatures, signature)
// 	}

// 	aggregatedSignature, err := signatures.Aggregate().Marshal()
// 	assert.NoError(t, err)

// 	signature := Signature{
// 		AggregatedSignature: aggregatedSignature,
// 		Bitmap:              bitmap,
// 	}

// 	assert.NoError(t, err)

// 	return &CommitmentMessageSigned{
// 		Message:      commitment,
// 		AggSignature: signature,
// 	}
// }

// func newBlockBuilderMock(buildBlock *StateBlock) *blockBuilderMock {
// 	mBlockBuilder := new(blockBuilderMock)
// 	mBlockBuilder.On("Build", mock.Anything).Return(buildBlock).Once()
// 	mBlockBuilder.On("Fill", mock.Anything).Return(nil).Once()
// 	return mBlockBuilder
// }

// func createTestUptimeCounter(t *testing.T, validatorSet AccountSet, epochSize uint64) *CommitEpoch {
// 	if validatorSet == nil {
// 		validatorSet = newTestValidators(5).getPublicIdentities()
// 	}

// 	uptime := Uptime{EpochID: 0}
// 	commitEpoch := &CommitEpoch{
// 		EpochID: 0,
// 		Epoch: Epoch{
// 			StartBlock: 1,
// 			EndBlock:   1 + epochSize,
// 			EpochRoot:  types.Hash{},
// 		},
// 		Uptime: uptime,
// 	}
// 	indexToStart := 0
// 	for i := uint64(0); i < epochSize; i++ {
// 		validatorIndex := indexToStart
// 		for j := 0; j < validatorSet.Len()-1; j++ {
// 			validatorIndex = validatorIndex % validatorSet.Len()
// 			uptime.addValidatorUptime(validatorSet[validatorIndex].Address, 1)
// 			validatorIndex++
// 		}

// 		indexToStart = (indexToStart + 1) % validatorSet.Len()
// 	}

// 	return commitEpoch
// }
