package polybft

import (
	"encoding/hex"
	"errors"
	"math"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"

	"github.com/umbracle/ethgo/abi"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestCheckpointManager_SubmitCheckpoint(t *testing.T) {
	t.Parallel()

	const (
		blocksCount = 10
		epochSize   = 2
	)

	var aliases = []string{"A", "B", "C", "D", "E"}

	validators := newTestValidatorsWithAliases(aliases)
	validatorsMetadata := validators.getPublicIdentities()
	txRelayerMock := newDummyTxRelayer(t)
	txRelayerMock.On("Call", mock.Anything, mock.Anything, mock.Anything).
		Return("2", error(nil)).
		Once()
	txRelayerMock.On("SendTransaction", mock.Anything, mock.Anything).
		Return(&ethgo.Receipt{Status: uint64(types.ReceiptSuccess)}, error(nil)).
		Times(4) // send transactions for checkpoint blocks: 4, 6, 8 (pending checkpoint blocks) and 10 (latest checkpoint block)

	backendMock := new(polybftBackendMock)
	backendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validatorsMetadata)

	var (
		headersMap  = &testHeadersMap{}
		epochNumber = uint64(1)
		dummyMsg    = []byte("checkpoint")
		idx         = uint64(0)
		header      *types.Header
		bitmap      bitmap.Bitmap
		signatures  bls.Signatures
	)

	validators.iterAcct(aliases, func(t *testValidator) {
		bitmap.Set(idx)
		signatures = append(signatures, t.mustSign(dummyMsg))
		idx++
	})

	signature, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)

	for i := uint64(1); i <= blocksCount; i++ {
		if i%epochSize == 1 {
			// epoch-beginning block
			checkpoint := &CheckpointData{
				BlockRound:  0,
				EpochNumber: epochNumber,
				EventRoot:   types.BytesToHash(generateRandomBytes(t)),
			}
			extra := createTestExtraObject(validatorsMetadata, validatorsMetadata, 3, 3, 3)
			extra.Checkpoint = checkpoint
			extra.Committed = &Signature{Bitmap: bitmap, AggregatedSignature: signature}
			header = &types.Header{
				ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
			}
			epochNumber++
		} else {
			header = header.Copy()
		}

		header.Number = i
		header.ComputeHash()
		headersMap.addHeader(header)
	}

	// mock blockchain
	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(headersMap.getHeader)

	validatorAcc := validators.getValidator("A")
	c := &checkpointManager{
		key:              wallet.NewEcdsaSigner(validatorAcc.Key()),
		txRelayer:        txRelayerMock,
		consensusBackend: backendMock,
		blockchain:       blockchainMock,
		logger:           hclog.NewNullLogger(),
	}

	err = c.submitCheckpoint(headersMap.getHeader(blocksCount), false)
	require.NoError(t, err)
	txRelayerMock.AssertExpectations(t)

	// make sure that expected blocks are checkpointed (epoch-ending ones)
	for _, checkpointBlock := range txRelayerMock.checkpointBlocks {
		header := headersMap.getHeader(checkpointBlock)
		require.NotNil(t, header)
		require.True(t, isEndOfPeriod(header.Number, epochSize))
	}
}

func TestCheckpointManager_abiEncodeCheckpointBlock(t *testing.T) {
	t.Parallel()

	const epochSize = uint64(10)

	currentValidators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D"})
	nextValidators := newTestValidatorsWithAliases([]string{"E", "F", "G", "H"})
	header := &types.Header{Number: 50}
	checkpoint := &CheckpointData{
		BlockRound:  1,
		EpochNumber: getEpochNumber(t, header.Number, epochSize),
		EventRoot:   types.BytesToHash(generateRandomBytes(t)),
	}

	proposalHash := generateRandomBytes(t)

	bmp := bitmap.Bitmap{}
	i := uint64(0)

	var signatures bls.Signatures

	currentValidators.iterAcct(nil, func(v *testValidator) {
		signatures = append(signatures, v.mustSign(proposalHash))
		bmp.Set(i)
		i++
	})

	aggSignature, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)

	extra := &Extra{Checkpoint: checkpoint}
	extra.Committed = &Signature{
		AggregatedSignature: aggSignature,
		Bitmap:              bmp,
	}
	header.ExtraData = append(make([]byte, signer.IstanbulExtraVanity), extra.MarshalRLPTo(nil)...)
	header.ComputeHash()

	backendMock := new(polybftBackendMock)
	backendMock.On("GetValidators", mock.Anything, mock.Anything).Return(currentValidators.getPublicIdentities())

	c := &checkpointManager{
		blockchain:       &blockchainMock{},
		consensusBackend: backendMock,
		logger:           hclog.NewNullLogger(),
	}
	checkpointDataEncoded, err := c.abiEncodeCheckpointBlock(header.Number, header.Hash, extra, nextValidators.getPublicIdentities())
	require.NoError(t, err)

	submit := &contractsapi.SubmitFunction{}
	require.NoError(t, submit.DecodeAbi(checkpointDataEncoded))

	require.Equal(t, new(big.Int).SetUint64(checkpoint.EpochNumber), submit.Checkpoint.Epoch)
	require.Equal(t, new(big.Int).SetUint64(header.Number), submit.Checkpoint.BlockNumber)
	require.Equal(t, checkpoint.EventRoot, submit.Checkpoint.EventRoot)
	require.Equal(t, new(big.Int).SetUint64(checkpoint.BlockRound), submit.CheckpointMetadata.BlockRound)
}

func TestCheckpointManager_getCurrentCheckpointID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		checkpointID string
		returnError  error
		errSubstring string
	}{
		{
			name:         "Happy path",
			checkpointID: "16",
			returnError:  error(nil),
			errSubstring: "",
		},
		{
			name:         "Rootchain call returns an error",
			checkpointID: "",
			returnError:  errors.New("internal error"),
			errSubstring: "failed to invoke currentCheckpointId function on the rootchain",
		},
		{
			name:         "Failed to parse return value from rootchain",
			checkpointID: "Hello World!",
			returnError:  error(nil),
			errSubstring: "failed to convert current checkpoint id",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			txRelayerMock := newDummyTxRelayer(t)
			txRelayerMock.On("Call", mock.Anything, mock.Anything, mock.Anything).
				Return(c.checkpointID, c.returnError).
				Once()

			checkpointMgr := &checkpointManager{
				txRelayer: txRelayerMock,
				key:       wallet.GenerateAccount().Ecdsa,
				logger:    hclog.NewNullLogger(),
			}
			actualCheckpointID, err := checkpointMgr.getLatestCheckpointBlock()
			if c.errSubstring == "" {
				expectedCheckpointID, err := strconv.ParseUint(c.checkpointID, 0, 64)
				require.NoError(t, err)
				require.Equal(t, expectedCheckpointID, actualCheckpointID)
			} else {
				require.ErrorContains(t, err, c.errSubstring)
			}

			txRelayerMock.AssertExpectations(t)
		})
	}
}

func TestCheckpointManager_IsCheckpointBlock(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		blockNumber        uint64
		checkpointsOffset  uint64
		isEpochEndingBlock bool
		isCheckpointBlock  bool
	}{
		{
			name:               "Not checkpoint block",
			blockNumber:        3,
			checkpointsOffset:  6,
			isEpochEndingBlock: false,
			isCheckpointBlock:  false,
		},
		{
			name:               "Checkpoint block",
			blockNumber:        6,
			checkpointsOffset:  6,
			isEpochEndingBlock: false,
			isCheckpointBlock:  true,
		},
		{
			name:               "Epoch ending block - Fixed epoch size met",
			blockNumber:        10,
			checkpointsOffset:  5,
			isEpochEndingBlock: true,
			isCheckpointBlock:  true,
		},
		{
			name:               "Epoch ending block - Epoch ended before fix size was met",
			blockNumber:        9,
			checkpointsOffset:  5,
			isEpochEndingBlock: true,
			isCheckpointBlock:  true,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			checkpointMgr := newCheckpointManager(wallet.NewEcdsaSigner(createTestKey(t)), c.checkpointsOffset, types.ZeroAddress, nil, nil, nil, hclog.NewNullLogger(), nil)
			require.Equal(t, c.isCheckpointBlock, checkpointMgr.isCheckpointBlock(c.blockNumber, c.isEpochEndingBlock))
		})
	}
}

func TestCheckpointManager_PostBlock(t *testing.T) {
	const (
		numOfReceipts = 5
		block         = 5
		epoch         = 1
	)

	state := newTestState(t)

	receipts := make([]*types.Receipt, numOfReceipts)
	for i := 0; i < numOfReceipts; i++ {
		receipts[i] = &types.Receipt{Logs: []*types.Log{
			createTestLogForExitEvent(t, uint64(i)),
		}}
	}

	req := &PostBlockRequest{FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: block}}, Receipts: receipts},
		Epoch: epoch}

	checkpointManager := newCheckpointManager(wallet.NewEcdsaSigner(createTestKey(t)), 5, types.ZeroAddress,
		nil, nil, nil, hclog.NewNullLogger(), state)

	t.Run("PostBlock - not epoch ending block", func(t *testing.T) {
		req.IsEpochEndingBlock = false
		require.NoError(t, checkpointManager.PostBlock(req))

		exitEvents, err := state.getExitEvents(epoch, func(exitEvent *ExitEvent) bool {
			return exitEvent.BlockNumber == block
		})

		require.NoError(t, err)
		require.Len(t, exitEvents, numOfReceipts)
		require.Equal(t, uint64(epoch), exitEvents[0].EpochNumber)
	})

	t.Run("PostBlock - epoch ending block (exit events are saved to the next epoch)", func(t *testing.T) {
		req.IsEpochEndingBlock = true
		require.NoError(t, checkpointManager.PostBlock(req))

		exitEvents, err := state.getExitEvents(epoch+1, func(exitEvent *ExitEvent) bool {
			return exitEvent.BlockNumber == block
		})

		require.NoError(t, err)
		require.Len(t, exitEvents, numOfReceipts)
		require.Equal(t, uint64(epoch+1), exitEvents[0].EpochNumber)
	})
}

func TestCheckpointManager_BuildEventRoot(t *testing.T) {
	t.Parallel()

	const (
		numOfBlocks         = 10
		numOfEventsPerBlock = 2
	)

	state := newTestState(t)
	checkpointManager := &checkpointManager{state: state}

	encodedEvents := setupExitEventsForProofVerification(t, state, numOfBlocks, numOfEventsPerBlock)

	t.Run("Get exit event root hash", func(t *testing.T) {
		t.Parallel()

		tree, err := NewMerkleTree(encodedEvents)
		require.NoError(t, err)

		hash, err := checkpointManager.BuildEventRoot(1)
		require.NoError(t, err)
		require.Equal(t, tree.Hash(), hash)
	})

	t.Run("Get exit event root hash - no events", func(t *testing.T) {
		t.Parallel()

		hash, err := checkpointManager.BuildEventRoot(2)
		require.NoError(t, err)
		require.Equal(t, types.Hash{}, hash)
	})
}

func TestCheckpointManager_GenerateExitProof(t *testing.T) {
	t.Parallel()

	const (
		numOfBlocks         = 10
		numOfEventsPerBlock = 2
	)

	state := newTestState(t)
	checkpointManager := &checkpointManager{
		state: state,
	}

	encodedEvents := setupExitEventsForProofVerification(t, state, numOfBlocks, numOfEventsPerBlock)
	checkpointEvents := encodedEvents[:numOfEventsPerBlock]

	// manually create merkle tree for a desired checkpoint to verify the generated proof
	tree, err := NewMerkleTree(checkpointEvents)
	require.NoError(t, err)

	proof, err := checkpointManager.GenerateExitProof(1, 1, 1)
	require.NoError(t, err)
	require.NotNil(t, proof)

	t.Run("Generate and validate exit proof", func(t *testing.T) {
		t.Parallel()
		// verify generated proof on desired tree
		require.NoError(t, VerifyProof(1, encodedEvents[1], proof.Proof, tree.Hash()))
	})

	t.Run("Generate and validate exit proof - invalid proof", func(t *testing.T) {
		t.Parallel()

		// copy and make proof invalid
		invalidProof := make([]types.Hash, len(proof.Proof))
		copy(invalidProof, proof.Proof)
		invalidProof[0][0]++

		// verify generated proof on desired tree
		require.ErrorContains(t, VerifyProof(1, encodedEvents[1], invalidProof, tree.Hash()), "not a member of merkle tree")
	})

	t.Run("Generate exit proof - no event", func(t *testing.T) {
		t.Parallel()

		_, err := checkpointManager.GenerateExitProof(21, 1, 1)
		require.ErrorContains(t, err, "could not find any exit event that has an id")
	})
}

func TestPerformExit(t *testing.T) {
	t.Parallel()

	//create validator set
	currentValidators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D"}, []uint64{100, 100, 100, 100})
	accSet := currentValidators.getPublicIdentities()

	senderAddress := types.Address{1}
	bn256Addr := types.Address{2}
	l1Cntract := types.Address{3}

	alloc := map[types.Address]*chain.GenesisAccount{
		senderAddress: {
			Balance: big.NewInt(100000000000),
		},
		contracts.BLSContract: {
			Code: contractsapi.BLS.DeployedBytecode,
		},
		bn256Addr: {
			Code: contractsapi.BLS256.DeployedBytecode,
		},
		l1Cntract: {
			Code: contractsapi.TestL1StateReceiver.DeployedBytecode,
		},
	}
	transition := newTestTransition(t, alloc)

	getField := func(addr types.Address, abi *abi.ABI, function string, args ...interface{}) []byte {
		input, err := abi.GetMethod(function).Encode(args)
		require.NoError(t, err)

		result := transition.Call2(senderAddress, addr, input, big.NewInt(0), 1000000000)
		require.NoError(t, result.Err)
		require.True(t, result.Succeeded())
		require.False(t, result.Failed())

		return result.ReturnValue
	}

	rootchainContractAddress := deployRootchainContract(t, transition, contractsapi.CheckpointManager, senderAddress, accSet, bn256Addr)
	exitHelperContractAddress := deployExitContract(t, transition, contractsapi.ExitHelper, senderAddress, rootchainContractAddress)

	require.Equal(t, getField(rootchainContractAddress, contractsapi.CheckpointManager.Abi, "currentCheckpointBlockNumber")[31], uint8(0))

	cm := checkpointManager{
		blockchain: &blockchainMock{},
	}
	accSetHash, err := accSet.Hash()
	require.NoError(t, err)

	blockHash := types.Hash{5}
	blockNumber := uint64(1)
	epochNumber := uint64(1)
	blockRound := uint64(1)

	exits := []*ExitEvent{
		{
			ID:       1,
			Sender:   ethgo.Address{7},
			Receiver: ethgo.Address(l1Cntract),
			Data:     []byte{123},
		},
		{
			ID:       2,
			Sender:   ethgo.Address{7},
			Receiver: ethgo.Address(l1Cntract),
			Data:     []byte{21},
		},
	}
	exitTrie, err := createExitTree(exits)
	require.NoError(t, err)

	eventRoot := exitTrie.Hash()

	checkpointData := CheckpointData{
		BlockRound:            blockRound,
		EpochNumber:           epochNumber,
		CurrentValidatorsHash: accSetHash,
		NextValidatorsHash:    accSetHash,
		EventRoot:             eventRoot,
	}

	checkpointHash, err := checkpointData.Hash(
		cm.blockchain.GetChainID(),
		blockRound,
		blockHash)
	require.NoError(t, err)

	bmp := bitmap.Bitmap{}
	i := uint64(0)

	var signatures bls.Signatures

	currentValidators.iterAcct(nil, func(v *testValidator) {
		signatures = append(signatures, v.mustSign(checkpointHash[:]))
		bmp.Set(i)
		i++
	})

	aggSignature, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)

	extra := &Extra{
		Checkpoint: &checkpointData,
	}
	extra.Committed = &Signature{
		AggregatedSignature: aggSignature,
		Bitmap:              bmp,
	}

	submitCheckpointEncoded, err := cm.abiEncodeCheckpointBlock(
		blockNumber,
		blockHash,
		extra,
		accSet)
	require.NoError(t, err)

	result := transition.Call2(senderAddress, rootchainContractAddress, submitCheckpointEncoded, big.NewInt(0), 1000000000)
	require.NoError(t, result.Err)
	require.True(t, result.Succeeded())
	require.False(t, result.Failed())
	require.Equal(t, getField(rootchainContractAddress, contractsapi.CheckpointManager.Abi, "currentCheckpointBlockNumber")[31], uint8(1))

	//check that the exit havent performed
	res := getField(exitHelperContractAddress, contractsapi.ExitHelper.Abi, "processedExits", exits[0].ID)
	require.Equal(t, int(res[31]), 0)

	proofExitEvent, err := ExitEventABIType.Encode(exits[0])
	require.NoError(t, err)
	proof, err := exitTrie.GenerateProofForLeaf(proofExitEvent, 0)
	require.NoError(t, err)
	leafIndex, err := exitTrie.LeafIndex(proofExitEvent)
	require.NoError(t, err)

	ehExit, err := contractsapi.ExitHelper.Abi.GetMethod("exit").Encode([]interface{}{
		blockNumber,
		leafIndex,
		proofExitEvent,
		proof,
	})
	require.NoError(t, err)

	result = transition.Call2(senderAddress, exitHelperContractAddress, ehExit, big.NewInt(0), 1000000000)
	require.NoError(t, result.Err)
	require.True(t, result.Succeeded())
	require.False(t, result.Failed())

	//check true
	res = getField(exitHelperContractAddress, contractsapi.ExitHelper.Abi, "processedExits", exits[0].ID)
	require.Equal(t, int(res[31]), 1)

	lastID := getField(l1Cntract, contractsapi.TestL1StateReceiver.Abi, "id")
	require.Equal(t, lastID[31], uint8(1))

	lastAddr := getField(l1Cntract, contractsapi.TestL1StateReceiver.Abi, "addr")
	require.Equal(t, exits[0].Sender[:], lastAddr[12:])

	lastCounter := getField(l1Cntract, contractsapi.TestL1StateReceiver.Abi, "counter")
	require.Equal(t, lastCounter[31], uint8(1))
}

func TestCommitEpoch(t *testing.T) {
	// init validator sets
	validatorSetSize := []int{5, 10, 50, 100, 150, 200}
	// number of delegators per validator
	delegPerVal := 100

	intialBalance := uint64(5 * math.Pow(10, 18))  // 5 tokens
	reward := uint64(math.Pow(10, 18))             // 1 token
	delegateAmount := uint64(math.Pow(10, 18)) / 2 // 0.5 token

	validatorSets := make([]*testValidators, len(validatorSetSize), len(validatorSetSize))

	// create all validator sets which will be used in test
	for i, size := range validatorSetSize {
		aliasses := make([]string, size, size)
		vps := make([]uint64, size, size)

		for j := 0; j < size; j++ {
			aliasses[j] = "v" + strconv.Itoa(j)
			vps[j] = intialBalance
		}

		validatorSets[i] = newTestValidatorsWithAliases(aliasses, vps)
	}

	// iterate through the validator set and do the test for each of them
	for _, currentValidators := range validatorSets {
		accSet := currentValidators.getPublicIdentities()
		valid2deleg := make(map[types.Address][]*wallet.Key, accSet.Len()) // delegators assigned to validators

		// add contracts to genesis data
		alloc := map[types.Address]*chain.GenesisAccount{
			contracts.ValidatorSetContract: {
				Code: contractsapi.ChildValidatorSet.DeployedBytecode,
			},
			contracts.BLSContract: {
				Code: contractsapi.BLS.DeployedBytecode,
			},
		}

		// validator data for polybft config
		initValidators := make([]*Validator, accSet.Len())

		for i, validator := range accSet {
			// add validator to genesis data
			alloc[validator.Address] = &chain.GenesisAccount{
				Balance: validator.VotingPower,
			}

			// create validator data for polybft config
			initValidators[i] = &Validator{
				Address: validator.Address,
				Balance: validator.VotingPower,
				BlsKey:  hex.EncodeToString(validator.BlsKey.Marshal()),
			}

			// create delegators
			delegWallets := createRandomTestKeys(t, delegPerVal)

			// add delegators to genesis data
			for j := 0; j < delegPerVal; j++ {
				delegator := delegWallets[j]
				alloc[types.Address(delegator.Address())] = &chain.GenesisAccount{
					Balance: new(big.Int).SetUint64(intialBalance),
				}
			}

			valid2deleg[validator.Address] = delegWallets
		}

		transition := newTestTransition(t, alloc)

		polyBFTConfig := PolyBFTConfig{
			InitialValidatorSet: initValidators,
			BlockTime:           2 * time.Second,
			EpochSize:           24 * 60 * 60 / 2,
			SprintSize:          5,
			EpochReward:         reward,
			// use 1st account as governance address
			Governance:       currentValidators.toValidatorSet().validators.GetAddresses()[0],
			ValidatorSetAddr: contracts.ValidatorSetContract,
		}

		// get data for ChildValidatorSet initialization
		initInput, err := getInitChildValidatorSetInput(polyBFTConfig)
		require.NoError(t, err)

		// init ChildValidatorSet
		err = initContract(contracts.ValidatorSetContract, initInput, "ChildValidatorSet", transition)
		require.NoError(t, err)

		// delegate amounts to validators
		for valAddress, delegators := range valid2deleg {
			for _, delegator := range delegators {
				encoded, err := contractsapi.ChildValidatorSet.Abi.Methods["delegate"].Encode(
					[]interface{}{valAddress, false})

				require.NoError(t, err)

				result := transition.Call2(types.Address(delegator.Address()), contracts.ValidatorSetContract, encoded, new(big.Int).SetUint64(delegateAmount), 1000000000000)
				require.False(t, result.Failed())
			}
		}

		// create input for commit epoch
		commitEpoch := createCommitEpoch(t, 1, accSet, polyBFTConfig.EpochSize)
		input, err := commitEpoch.EncodeAbi()
		require.NoError(t, err)

		// call commit epoch
		result := transition.Call2(contracts.SystemCaller, contracts.ValidatorSetContract, input, big.NewInt(0), 10000000000)

		t.Logf("Number of validators %d when we add %d of delegators, Gas used %+v\n", accSet.Len(), accSet.Len()*delegPerVal, result.GasUsed)

		commitEpoch = createCommitEpoch(t, 2, accSet, polyBFTConfig.EpochSize)
		input, err = commitEpoch.EncodeAbi()
		require.NoError(t, err)

		// call commit epoch
		result = transition.Call2(contracts.SystemCaller, contracts.ValidatorSetContract, input, big.NewInt(0), 10000000000)
		t.Logf("Number of validators %d, Number of delegator %d, Gas used %+v\n", accSet.Len(), accSet.Len()*delegPerVal, result.GasUsed)
	}
}

func createCommitEpoch(t *testing.T, epochID uint64, validatorSet AccountSet, epochSize uint64) *CommitEpoch {
	t.Helper()

	var startBlock uint64 = 0
	if epochID > 1 {
		startBlock = (epochID - 1) * epochSize
	}

	uptime := Uptime{EpochID: epochID}

	commitEpoch := &CommitEpoch{
		EpochID: uptime.EpochID,
		Epoch: Epoch{
			StartBlock: startBlock + 1,
			EndBlock:   epochSize * epochID,
			EpochRoot:  types.Hash{},
		},
		Uptime: uptime,
	}

	for i := range validatorSet {
		uptime.addValidatorUptime(validatorSet[i].Address, epochSize)
	}

	return commitEpoch
}

func deployRootchainContract(t *testing.T, transition *state.Transition, rootchainArtifact *artifact.Artifact, sender types.Address, accSet AccountSet, bn256Addr types.Address) types.Address {
	t.Helper()

	result := transition.Create2(sender, rootchainArtifact.Bytecode, big.NewInt(0), 1000000000)
	assert.NoError(t, result.Err)
	rcAddress := result.Address

	init, err := rootchainArtifact.Abi.GetMethod("initialize").Encode([4]interface{}{
		contracts.BLSContract,
		bn256Addr,
		bls.GetDomain(),
		accSet.ToAPIBinding()})
	if err != nil {
		t.Fatal(err)
	}

	result = transition.Call2(sender, rcAddress, init, big.NewInt(0), 1000000000)
	require.True(t, result.Succeeded())
	require.False(t, result.Failed())
	require.NoError(t, result.Err)

	getDomain, err := rootchainArtifact.Abi.GetMethod("domain").Encode([]interface{}{})
	require.NoError(t, err)

	result = transition.Call2(sender, rcAddress, getDomain, big.NewInt(0), 1000000000)
	require.Equal(t, result.ReturnValue, bls.GetDomain())

	return rcAddress
}

func deployExitContract(t *testing.T, transition *state.Transition, exitHelperArtifcat *artifact.Artifact, sender types.Address, rootchainContractAddress types.Address) types.Address {
	t.Helper()

	result := transition.Create2(sender, exitHelperArtifcat.Bytecode, big.NewInt(0), 1000000000)
	assert.NoError(t, result.Err)
	ehAddress := result.Address

	ehInit, err := exitHelperArtifcat.Abi.GetMethod("initialize").Encode([]interface{}{ethgo.Address(rootchainContractAddress)})
	require.NoError(t, err)

	result = transition.Call2(sender, ehAddress, ehInit, big.NewInt(0), 1000000000)
	require.NoError(t, result.Err)
	require.True(t, result.Succeeded())
	require.False(t, result.Failed())

	return ehAddress
}

var _ txrelayer.TxRelayer = (*dummyTxRelayer)(nil)

type dummyTxRelayer struct {
	mock.Mock

	test             *testing.T
	checkpointBlocks []uint64
}

func newDummyTxRelayer(t *testing.T) *dummyTxRelayer {
	t.Helper()

	return &dummyTxRelayer{test: t}
}

func (d dummyTxRelayer) Call(from ethgo.Address, to ethgo.Address, input []byte) (string, error) {
	args := d.Called(from, to, input)

	return args.String(0), args.Error(1)
}

func (d *dummyTxRelayer) SendTransaction(transaction *ethgo.Transaction, key ethgo.Key) (*ethgo.Receipt, error) {
	blockNumber := getBlockNumberCheckpointSubmitInput(d.test, transaction.Input)
	d.checkpointBlocks = append(d.checkpointBlocks, blockNumber)
	args := d.Called(transaction, key)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

// SendTransactionLocal sends non-signed transaction (this is only for testing purposes)
func (d *dummyTxRelayer) SendTransactionLocal(txn *ethgo.Transaction) (*ethgo.Receipt, error) {
	args := d.Called(txn)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

func getBlockNumberCheckpointSubmitInput(t *testing.T, input []byte) uint64 {
	t.Helper()

	submit := &contractsapi.SubmitFunction{}
	require.NoError(t, submit.DecodeAbi(input))

	return submit.Checkpoint.BlockNumber.Uint64()
}

func createTestLogForExitEvent(t *testing.T, exitEventID uint64) *types.Log {
	t.Helper()

	topics := make([]types.Hash, 4)
	topics[0] = types.Hash(exitEventABI.ID())
	topics[1] = types.BytesToHash(itob(exitEventID))
	topics[2] = types.BytesToHash(types.StringToAddress("0x1111").Bytes())
	topics[3] = types.BytesToHash(types.StringToAddress("0x2222").Bytes())
	someType := abi.MustNewType("tuple(string firstName, string lastName)")
	encodedData, err := someType.Encode(map[string]string{"firstName": "John", "lastName": "Doe"})
	require.NoError(t, err)

	return &types.Log{
		Address: contracts.L2StateSenderContract,
		Topics:  topics,
		Data:    encodedData,
	}
}
