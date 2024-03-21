package finalize

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/helper"
	polybftsecrets "github.com/0xPolygon/polygon-edge/command/secrets/init"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
)

var (
	params               finalizeParams
	genesisSetABIFn      = contractsapi.BladeManager.Abi.Methods["genesisSet"]
	finalizeGenesisABIFn = contractsapi.BladeManager.Abi.Methods["finalizeGenesis"]
)

func GetCommand() *cobra.Command {
	registerCmd := &cobra.Command{
		Use:     "finalize-bridge",
		Short:   "Blade bridge initialization & finalization command",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	setFlags(registerCmd)

	return registerCmd
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPC = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.accountDir,
		polybftsecrets.AccountDirFlag,
		"",
		polybftsecrets.AccountDirFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.accountConfig,
		polybftsecrets.AccountConfigFlag,
		"",
		polybftsecrets.AccountConfigFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.privateKey,
		polybftsecrets.PrivateKeyFlag,
		"",
		polybftsecrets.PrivateKeyFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.genesisPath,
		bridgeHelper.GenesisPathFlag,
		bridgeHelper.DefaultGenesisPath,
		bridgeHelper.GenesisPathFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.bladeManager,
		bridgeHelper.BladeManagerFlag,
		"",
		bridgeHelper.BladeManagerFlagDesc,
	)

	cmd.Flags().DurationVar(
		&params.txTimeout,
		bridgeHelper.TxTimeoutFlag,
		5*time.Second,
		"timeout for receipts in milliseconds",
	)

	cmd.Flags().DurationVar(
		&params.txPollFreq,
		bridgeHelper.TxPollFreqFlag,
		50,
		"frequency in milliseconds for poll transactions",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountDirFlag)
	_ = cmd.MarkFlagRequired(bridgeHelper.BladeManagerFlag)

	helper.RegisterJSONRPCFlag(cmd)
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ownerKey, err := bridgeHelper.GetECDSAKey(params.privateKey, params.accountDir, params.accountConfig)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC),
		txrelayer.WithReceiptsTimeout(params.txTimeout),
		txrelayer.WithReceiptsPollFreq(params.txPollFreq))
	if err != nil {
		return fmt.Errorf("enlist validator failed: %w", err)
	}

	bladeManagerAddr := params.bladeManagerAddr

	// finalize genesis accounts on BladeManager so that no one can stake and premine no more
	encoded, err := finalizeGenesisABIFn.Encode([]interface{}{})
	if err != nil {
		return err
	}

	txn := bridgeHelper.CreateTransaction(ownerKey.Address(), &bladeManagerAddr, encoded, nil, true)

	if _, err = txRelayer.Call(ownerKey.Address(), bladeManagerAddr, encoded); err == nil {
		receipt, err := txRelayer.SendTransaction(txn, ownerKey)
		if err != nil {
			return fmt.Errorf("finalizing genesis validator set failed. Error: %w", err)
		}

		if receipt.Status == uint64(types.ReceiptFailed) {
			return fmt.Errorf("finalizing genesis validator set transaction failed on block %d", receipt.BlockNumber)
		}
	} else if !strings.Contains(err.Error(), "execution reverted: GenesisLib: already finalized") {
		return err
	}

	// get genesis config
	chainConfig, err := chain.ImportFromFile(params.genesisPath)
	if err != nil {
		return fmt.Errorf("failed to read chain configuration: %w", err)
	}

	consensusConfig, err := polybft.GetPolyBFTConfig(chainConfig.Params)
	if err != nil {
		return fmt.Errorf("failed to retrieve consensus configuration: %w", err)
	}

	// get genesis account set from BladeManager
	genesisSetInput, err := genesisSetABIFn.Encode([]interface{}{})
	if err != nil {
		return fmt.Errorf("failed to encode genesis set input: %w", err)
	}

	genesisSetHexOut, err := txRelayer.Call(types.ZeroAddress, bladeManagerAddr, genesisSetInput)
	if err != nil {
		return fmt.Errorf("failed to retrieve genesis set: %w", err)
	}

	genesisAccounts, err := decodeGenesisAccounts(genesisSetHexOut)
	if err != nil {
		return err
	}

	// add to premine allocs what was premined on BladeManager
	for addr, genesisAcc := range genesisAccounts {
		chainConfig.Genesis.Alloc[addr] = &chain.GenesisAccount{
			Balance: new(big.Int).Add(genesisAcc.PreminedTokens, genesisAcc.StakedTokens),
		}
	}

	validatorMetadata := make([]*validator.ValidatorMetadata, len(consensusConfig.InitialValidatorSet))

	// check what the validators staked and update them accordingly
	for i, val := range consensusConfig.InitialValidatorSet {
		if genesisAcc, exists := genesisAccounts[val.Address]; exists {
			val.Stake = genesisAcc.StakedTokens
		} else {
			return fmt.Errorf("genesis validator %v does not exist in genesis validator set on BladeManager",
				val.Address)
		}

		metadata, err := val.ToValidatorMetadata()
		if err != nil {
			return err
		}

		validatorMetadata[i] = metadata
	}

	// update the voting power in genesis block extra
	// based on finalized stake on rootchain
	genesisExtraData, err := genesis.GenerateExtraDataPolyBft(validatorMetadata)
	if err != nil {
		return err
	}

	chainConfig.Genesis.ExtraData = genesisExtraData
	chainConfig.Params.Engine[polybft.ConsensusName] = consensusConfig

	// save updated stake and genesis extra to genesis file on disk
	if err := helper.WriteGenesisConfigToDisk(chainConfig, params.genesisPath); err != nil {
		return fmt.Errorf("failed to save chain configuration bridge data: %w", err)
	}

	// initialize CheckpointManager contract since it needs to have a valid VotingPowers of validators
	if err := initializeCheckpointManager(outputter, txRelayer,
		consensusConfig, chainConfig.Params.ChainID, ownerKey); err != nil {
		return fmt.Errorf("could not initialize CheckpointManager with finalized genesis validator set: %w", err)
	}

	return nil
}

// decodeGenesisAccounts decodes genesis set retrieved from CustomSupernetManager contract
func decodeGenesisAccounts(genesisSetRaw string) (map[types.Address]*contractsapi.GenesisAccount, error) {
	decodeAccount := func(rawAccount map[string]interface{}) (*contractsapi.GenesisAccount, error) {
		addr, ok := rawAccount["addr"].(ethgo.Address)
		if !ok {
			return nil, errors.New("failed to retrieve genesis account address")
		}

		preminedTokens, ok := rawAccount["preminedTokens"].(*big.Int)
		if !ok {
			return nil, errors.New("failed to retrieve genesis account non-staked balance")
		}

		stakedTokens, ok := rawAccount["stakedTokens"].(*big.Int)
		if !ok {
			return nil, errors.New("failed to retrieve genesis account staked balance")
		}

		isValidator, ok := rawAccount["isValidator"].(bool)
		if !ok {
			return nil, errors.New("failed to retrieve genesis account isValidator indication")
		}

		return &contractsapi.GenesisAccount{
			Addr:           types.Address(addr),
			PreminedTokens: preminedTokens,
			StakedTokens:   stakedTokens,
			IsValidator:    isValidator,
		}, nil
	}

	genesisSetRawOut, err := hex.DecodeHex(genesisSetRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to decode genesis set from hex format: %w", err)
	}

	decodedGenesisSet, err := genesisSetABIFn.Outputs.Decode(genesisSetRawOut)
	if err != nil {
		return nil, fmt.Errorf("failed to decode genesis set from raw format: %w", err)
	}

	decodedGenesisSetMap, ok := decodedGenesisSet.(map[string]interface{})
	if !ok {
		return nil, errors.New("failed to convert genesis set to map")
	}

	decodedGenesisSetSliceMap, ok := decodedGenesisSetMap["0"].([]map[string]interface{})
	if !ok {
		return nil, errors.New("failed to convert genesis set to slice")
	}

	genesisAccounts := make(map[types.Address]*contractsapi.GenesisAccount, len(decodedGenesisSetSliceMap))

	for _, rawGenesisAccount := range decodedGenesisSetSliceMap {
		decodedAccount, err := decodeAccount(rawGenesisAccount)
		if err != nil {
			return nil, err
		}

		genesisAccounts[decodedAccount.Addr] = decodedAccount
	}

	return genesisAccounts, nil
}

// validatorSetToABISlice converts given validators to generic map
// which is used for ABI encoding validator set being sent to the rootchain contract
func validatorSetToABISlice(o command.OutputFormatter,
	validators []*validator.GenesisValidator) ([]*contractsapi.Validator, error) {
	accSet := make(validator.AccountSet, len(validators))

	if _, err := o.Write([]byte("[VALIDATORS - CHECKPOINT MANAGER] \n")); err != nil {
		return nil, err
	}

	for i, val := range validators {
		if _, err := o.Write([]byte(fmt.Sprintf("%v\n", val))); err != nil {
			return nil, err
		}

		blsKey, err := val.UnmarshalBLSPublicKey()
		if err != nil {
			return nil, err
		}

		accSet[i] = &validator.ValidatorMetadata{
			Address:     val.Address,
			BlsKey:      blsKey,
			VotingPower: new(big.Int).Set(val.Stake),
		}
	}

	hash, err := accSet.Hash()
	if err != nil {
		return nil, err
	}

	if _, err := o.Write([]byte(
		fmt.Sprintf("[VALIDATORS - CHECKPOINT MANAGER] Validators hash: %s\n", hash))); err != nil {
		return nil, err
	}

	return accSet.ToAPIBinding(), nil
}

// initializeCheckpointManager initializes CheckpointManager contract on rootchain
// based on finalized stake (voting power) of genesis validators on root
func initializeCheckpointManager(outputter command.OutputFormatter,
	txRelayer txrelayer.TxRelayer,
	consensusConfig polybft.PolyBFTConfig, chainID int64,
	deployerKey crypto.Key) error {
	validatorSet, err := validatorSetToABISlice(outputter, consensusConfig.InitialValidatorSet)
	if err != nil {
		return fmt.Errorf("failed to convert validators to map: %w", err)
	}

	initParams := &contractsapi.InitializeCheckpointManagerFn{
		ChainID_:        big.NewInt(chainID),
		NewBls:          consensusConfig.Bridge.BLSAddress,
		NewBn256G2:      consensusConfig.Bridge.BN256G2Address,
		NewValidatorSet: validatorSet,
	}

	input, err := initParams.EncodeAbi()
	if err != nil {
		return fmt.Errorf("failed to encode initialization params for CheckpointManager.initialize. error: %w", err)
	}

	if _, err := bridgeHelper.SendTransaction(txRelayer, consensusConfig.Bridge.CheckpointManagerAddr,
		input, "CheckpointManager", deployerKey); err != nil {
		return err
	}

	outputter.WriteCommandResult(
		&bridgeHelper.MessageResult{
			Message: fmt.Sprintf("CheckpointManager contract is initialized"),
		})

	return nil
}
