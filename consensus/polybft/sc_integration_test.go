package polybft

import (
	"math"
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestIntegration_PerformExit(t *testing.T) {
	t.Parallel()

	const gasLimit = 1000000000000

	// create validator set and checkpoint mngr
	currentValidators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D"}, []uint64{100, 100, 100, 100})
	accSet := currentValidators.GetPublicIdentities()
	cm := checkpointManager{blockchain: &blockchainMock{}}

	deployerAddress := types.Address{76, 76, 1} // account that will deploy contracts
	senderAddress := types.Address{1}           // account that sends exit/withdraw transactions
	receiverAddr := types.Address{6}            // account that receive tokens
	amount1 := big.NewInt(3)                    // amount of the first widrawal
	amount2 := big.NewInt(2)                    // amount of the second widrawal
	bn256Addr := types.Address{2}               // bls contract
	stateSenderAddr := types.Address{5}         // generic bridge contract on rootchain

	alloc := map[types.Address]*chain.GenesisAccount{
		senderAddress:         {Balance: new(big.Int).Add(amount1, amount2)}, // give some ethers to sender
		deployerAddress:       {Balance: ethgo.Ether(100)},                   // give 100 ethers to deployer
		contracts.BLSContract: {Code: contractsapi.BLS.DeployedBytecode},
		bn256Addr:             {Code: contractsapi.BLS256.DeployedBytecode},
		stateSenderAddr:       {Code: contractsapi.StateSender.DeployedBytecode},
	}
	transition := newTestTransition(t, alloc)

	getField := func(addr types.Address, abi *abi.ABI, function string, args ...interface{}) []byte {
		input, err := abi.GetMethod(function).Encode(args)
		require.NoError(t, err)

		result := transition.Call2(deployerAddress, addr, input, big.NewInt(0), gasLimit)
		require.True(t, result.Succeeded())

		return result.ReturnValue
	}

	// deploy MockERC20 as root chain ERC 20 token
	rootERC20Addr := deployAndInitContract(t, transition, contractsapi.RootERC20.Bytecode, deployerAddress, nil)

	// deploy CheckpointManager
	checkpointManagerInit := func() ([]byte, error) {
		return (&contractsapi.InitializeCheckpointManagerFn{
			NewBls:          contracts.BLSContract,
			NewBn256G2:      bn256Addr,
			NewValidatorSet: accSet.ToAPIBinding(),
			ChainID_:        big.NewInt(0),
		}).EncodeAbi()
	}

	checkpointMgrConstructor := &contractsapi.CheckpointManagerConstructorFn{Initiator: deployerAddress}
	constructorInput, err := checkpointMgrConstructor.EncodeAbi()
	require.NoError(t, err)

	checkpointManagerAddr := deployAndInitContract(t, transition, append(contractsapi.CheckpointManager.Bytecode, constructorInput...), deployerAddress, checkpointManagerInit)

	// deploy ExitHelper
	exitHelperInit := func() ([]byte, error) {
		return (&contractsapi.InitializeExitHelperFn{NewCheckpointManager: checkpointManagerAddr}).EncodeAbi()
	}
	exitHelperContractAddress := deployAndInitContract(t, transition, contractsapi.ExitHelper.Bytecode, deployerAddress, exitHelperInit)

	// deploy RootERC20Predicate
	rootERC20PredicateInit := func() ([]byte, error) {
		return (&contractsapi.InitializeRootERC20PredicateFn{
			NewStateSender:         stateSenderAddr,
			NewExitHelper:          exitHelperContractAddress,
			NewChildERC20Predicate: contracts.ChildERC20PredicateContract,
			NewChildTokenTemplate:  contracts.ChildERC20Contract,
			NewNativeTokenRoot:     contracts.NativeERC20TokenContract,
		}).EncodeAbi()
	}
	rootERC20PredicateAddr := deployAndInitContract(t, transition, contractsapi.RootERC20Predicate.Bytecode, deployerAddress, rootERC20PredicateInit)

	// validate initialization of CheckpointManager
	require.Equal(t, getField(checkpointManagerAddr, contractsapi.CheckpointManager.Abi, "currentCheckpointBlockNumber")[31], uint8(0))

	accSetHash, err := accSet.Hash()
	require.NoError(t, err)

	blockHash := types.Hash{5}
	blockNumber := uint64(1)
	epochNumber := uint64(1)
	blockRound := uint64(1)

	// mint
	mintInput, err := (&contractsapi.MintRootERC20Fn{
		To:     senderAddress,
		Amount: alloc[senderAddress].Balance,
	}).EncodeAbi()
	require.NoError(t, err)

	result := transition.Call2(deployerAddress, rootERC20Addr, mintInput, nil, gasLimit)
	require.NoError(t, result.Err)

	// approve
	approveInput, err := (&contractsapi.ApproveRootERC20Fn{
		Spender: rootERC20PredicateAddr,
		Amount:  alloc[senderAddress].Balance,
	}).EncodeAbi()
	require.NoError(t, err)

	result = transition.Call2(senderAddress, rootERC20Addr, approveInput, big.NewInt(0), gasLimit)
	require.NoError(t, result.Err)

	// deposit
	depositInput, err := (&contractsapi.DepositToRootERC20PredicateFn{
		RootToken: rootERC20Addr,
		Receiver:  receiverAddr,
		Amount:    new(big.Int).Add(amount1, amount2),
	}).EncodeAbi()
	require.NoError(t, err)

	// send sync events to childchain so that receiver can obtain tokens
	result = transition.Call2(senderAddress, rootERC20PredicateAddr, depositInput, big.NewInt(0), gasLimit)
	require.NoError(t, result.Err)

	// simulate withdrawal from childchain to rootchain
	widthdrawSig := crypto.Keccak256([]byte("WITHDRAW"))
	erc20DataType := abi.MustNewType(
		"tuple(bytes32 withdrawSignature, address rootToken, address withdrawer, address receiver, uint256 amount)")

	exitData1, err := erc20DataType.Encode(map[string]interface{}{
		"withdrawSignature": widthdrawSig,
		"rootToken":         ethgo.Address(rootERC20Addr),
		"withdrawer":        ethgo.Address(senderAddress),
		"receiver":          ethgo.Address(receiverAddr),
		"amount":            amount1,
	})
	require.NoError(t, err)

	exitData2, err := erc20DataType.Encode(map[string]interface{}{
		"withdrawSignature": widthdrawSig,
		"rootToken":         ethgo.Address(rootERC20Addr),
		"withdrawer":        ethgo.Address(senderAddress),
		"receiver":          ethgo.Address(receiverAddr),
		"amount":            amount2,
	})
	require.NoError(t, err)

	exits := []*ExitEvent{
		{
			L2StateSyncedEvent: &contractsapi.L2StateSyncedEvent{
				ID:       big.NewInt(1),
				Sender:   contracts.ChildERC20PredicateContract,
				Receiver: rootERC20PredicateAddr,
				Data:     exitData1,
			},
		},
		{
			L2StateSyncedEvent: &contractsapi.L2StateSyncedEvent{
				ID:       big.NewInt(2),
				Sender:   contracts.ChildERC20PredicateContract,
				Receiver: rootERC20PredicateAddr,
				Data:     exitData2,
			},
		},
	}
	exitTree, err := createExitTree(exits)
	require.NoError(t, err)

	eventRoot := exitTree.Hash()

	checkpointData := &CheckpointData{
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

	i := uint64(0)
	bmp := bitmap.Bitmap{}
	signatures := bls.Signatures(nil)

	currentValidators.IterAcct(nil, func(v *validator.TestValidator) {
		signatures = append(signatures, v.MustSign(checkpointHash[:], signer.DomainCheckpointManager))
		bmp.Set(i)
		i++
	})

	aggSignature, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)

	extra := &Extra{
		Checkpoint: checkpointData,
		Committed: &Signature{
			AggregatedSignature: aggSignature,
			Bitmap:              bmp,
		},
	}

	// submit a checkpoint
	submitCheckpointEncoded, err := cm.abiEncodeCheckpointBlock(blockNumber, blockHash, extra, accSet)
	require.NoError(t, err)

	result = transition.Call2(senderAddress, checkpointManagerAddr, submitCheckpointEncoded, big.NewInt(0), gasLimit)
	require.NoError(t, result.Err)
	require.Equal(t, getField(checkpointManagerAddr, contractsapi.CheckpointManager.Abi, "currentCheckpointBlockNumber")[31], uint8(1))

	// check that the exit hasn't performed
	res := getField(exitHelperContractAddress, contractsapi.ExitHelper.Abi, "processedExits", exits[0].ID)
	require.Equal(t, 0, int(res[31]))

	proofExitEvent, err := exits[0].L2StateSyncedEvent.Encode()
	require.NoError(t, err)

	proof, err := exitTree.GenerateProof(proofExitEvent)
	require.NoError(t, err)

	leafIndex, err := exitTree.LeafIndex(proofExitEvent)
	require.NoError(t, err)

	exitFnInput, err := (&contractsapi.ExitExitHelperFn{
		BlockNumber:  new(big.Int).SetUint64(blockNumber),
		LeafIndex:    new(big.Int).SetUint64(leafIndex),
		UnhashedLeaf: proofExitEvent,
		Proof:        proof,
	}).EncodeAbi()
	require.NoError(t, err)

	result = transition.Call2(senderAddress, exitHelperContractAddress, exitFnInput, big.NewInt(0), gasLimit)
	require.NoError(t, result.Err)

	// check that first exit event is processed
	res = getField(exitHelperContractAddress, contractsapi.ExitHelper.Abi, "processedExits", exits[0].ID)
	require.Equal(t, 1, int(res[31]))

	res = getField(rootERC20Addr, contractsapi.RootERC20.Abi, "balanceOf", receiverAddr)
	require.Equal(t, amount1, new(big.Int).SetBytes(res))
}

func TestIntegration_CommitEpoch(t *testing.T) {
	t.Parallel()

	// init validator sets
	validatorSetSize := []int{5, 10, 50, 100}

	intialBalance := uint64(5 * math.Pow(10, 18)) // 5 tokens
	reward := uint64(math.Pow(10, 18))            // 1 token
	walletAddress := types.StringToAddress("1234889893")

	validatorSets := make([]*validator.TestValidators, len(validatorSetSize), len(validatorSetSize))

	// create all validator sets which will be used in test
	for i, size := range validatorSetSize {
		aliases := make([]string, size, size)
		vps := make([]uint64, size, size)

		for j := 0; j < size; j++ {
			aliases[j] = "v" + strconv.Itoa(j)
			vps[j] = intialBalance
		}

		validatorSets[i] = validator.NewTestValidatorsWithAliases(t, aliases, vps)
	}

	// iterate through the validator set and do the test for each of them
	for _, currentValidators := range validatorSets {
		accSet := currentValidators.GetPublicIdentities()

		// validator data for polybft config
		initValidators := make([]*validator.GenesisValidator, accSet.Len())
		// add contracts to genesis data
		alloc := map[types.Address]*chain.GenesisAccount{
			contracts.ValidatorSetContract: {
				Code: contractsapi.ValidatorSet.DeployedBytecode,
			},
			contracts.RewardPoolContract: {
				Code: contractsapi.RewardPool.DeployedBytecode,
			},
			contracts.NativeERC20TokenContract: {
				Code: contractsapi.NativeERC20.DeployedBytecode,
			},
			walletAddress: {
				Balance: new(big.Int).SetUint64(intialBalance),
			},
		}

		for i, val := range accSet {
			// add validator to genesis data
			alloc[val.Address] = &chain.GenesisAccount{
				Balance: val.VotingPower,
			}

			// create validator data for polybft config
			initValidators[i] = &validator.GenesisValidator{
				Address: val.Address,
				Stake:   val.VotingPower,
				BlsKey:  hex.EncodeToString(val.BlsKey.Marshal()),
			}
		}

		polyBFTConfig := PolyBFTConfig{
			InitialValidatorSet: initValidators,
			EpochSize:           24 * 60 * 60 / 2,
			SprintSize:          5,
			EpochReward:         reward,
			// use 1st account as governance address
			Governance: currentValidators.ToValidatorSet().Accounts().GetAddresses()[0],
			RewardConfig: &RewardsConfig{
				TokenAddress:  contracts.NativeERC20TokenContract,
				WalletAddress: walletAddress,
				WalletAmount:  new(big.Int).SetUint64(intialBalance),
			},
			Bridge: &BridgeConfig{
				CustomSupernetManagerAddr: types.StringToAddress("0x12312451"),
			},
		}

		transition := newTestTransition(t, alloc)

		// init ValidatorSet
		err := initValidatorSet(polyBFTConfig, transition)
		require.NoError(t, err)

		// init RewardPool
		err = initRewardPool(polyBFTConfig, transition)
		require.NoError(t, err)

		// approve reward pool as reward token spender
		err = approveRewardPoolAsSpender(polyBFTConfig, transition)
		require.NoError(t, err)

		// create input for commit epoch
		commitEpoch := createTestCommitEpochInput(t, 1, polyBFTConfig.EpochSize)
		input, err := commitEpoch.EncodeAbi()
		require.NoError(t, err)

		// call commit epoch
		result := transition.Call2(contracts.SystemCaller, contracts.ValidatorSetContract, input, big.NewInt(0), 10000000000)
		require.NoError(t, result.Err)
		t.Logf("Number of validators %d on commit epoch, Gas used %+v\n", accSet.Len(), result.GasUsed)

		// create input for distribute rewards
		distributeRewards := createTestDistributeRewardsInput(t, 1, accSet, polyBFTConfig.EpochSize)
		input, err = distributeRewards.EncodeAbi()
		require.NoError(t, err)

		// call reward distributor
		result = transition.Call2(contracts.SystemCaller, contracts.RewardPoolContract, input, big.NewInt(0), 10000000000)
		require.NoError(t, result.Err)
		t.Logf("Number of validators %d on reward distribution, Gas used %+v\n", accSet.Len(), result.GasUsed)

		commitEpoch = createTestCommitEpochInput(t, 2, polyBFTConfig.EpochSize)
		input, err = commitEpoch.EncodeAbi()
		require.NoError(t, err)

		// call commit epoch
		result = transition.Call2(contracts.SystemCaller, contracts.ValidatorSetContract, input, big.NewInt(0), 10000000000)
		require.NoError(t, result.Err)
		t.Logf("Number of validators %d on commit epoch, Gas used %+v\n", accSet.Len(), result.GasUsed)

		distributeRewards = createTestDistributeRewardsInput(t, 2, accSet, polyBFTConfig.EpochSize)
		input, err = distributeRewards.EncodeAbi()
		require.NoError(t, err)

		// call reward distributor
		result = transition.Call2(contracts.SystemCaller, contracts.RewardPoolContract, input, big.NewInt(0), 10000000000)
		require.NoError(t, result.Err)
		t.Logf("Number of validators %d on reward distribution, Gas used %+v\n", accSet.Len(), result.GasUsed)
	}
}

func deployAndInitContract(t *testing.T, transition *state.Transition, bytecode []byte, sender types.Address,
	initCallback func() ([]byte, error)) types.Address {
	t.Helper()

	deployResult := transition.Create2(sender, bytecode, big.NewInt(0), 1e9)
	assert.NoError(t, deployResult.Err)

	if initCallback != nil {
		initInput, err := initCallback()
		require.NoError(t, err)

		result := transition.Call2(sender, deployResult.Address, initInput, big.NewInt(0), 1e9)
		require.NoError(t, result.Err)
	}

	return deployResult.Address
}
