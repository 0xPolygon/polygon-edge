package supernet

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	params               supernetParams
	genesisSetABIFn      = contractsapi.CustomSupernetManager.Abi.Methods["genesisSet"]
	genesisBalancesABIFn = contractsapi.CustomSupernetManager.Abi.Methods["genesisBalances"]
)

func GetCommand() *cobra.Command {
	registerCmd := &cobra.Command{
		Use:     "supernet",
		Short:   "Supernet initialization & finalization command",
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
		rootHelper.GenesisPathFlag,
		rootHelper.DefaultGenesisPath,
		rootHelper.GenesisPathFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.supernetManagerAddress,
		rootHelper.SupernetManagerFlag,
		"",
		rootHelper.SupernetManagerFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.stakeManagerAddress,
		rootHelper.StakeManagerFlag,
		"",
		fmt.Sprintf("[DEPRECATED] %s", rootHelper.StakeManagerFlagDesc),
	)

	cmd.Flags().BoolVar(
		&params.finalizeGenesisSet,
		finalizeGenesisSetFlag,
		false,
		"indicates if genesis validator set should be finalized on rootchain",
	)

	cmd.Flags().BoolVar(
		&params.enableStaking,
		enableStakingFlag,
		false,
		"indicates if staking will be enabled after finalization of genesis validators on rootchain",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.AccountDirFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountConfigFlag)
	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.PrivateKeyFlag, polybftsecrets.AccountDirFlag)

	helper.RegisterJSONRPCFlag(cmd)
}

func runCommand(cmd *cobra.Command, _ []string) error {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ownerKey, err := rootHelper.GetECDSAKey(params.privateKey, params.accountDir, params.accountConfig)
	if err != nil {
		return err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPC))
	if err != nil {
		return fmt.Errorf("enlist validator failed: %w", err)
	}

	supernetAddr := ethgo.Address(types.StringToAddress(params.supernetManagerAddress))

	if params.finalizeGenesisSet {
		encoded, err := contractsapi.CustomSupernetManager.Abi.Methods["finalizeGenesis"].Encode([]interface{}{})
		if err != nil {
			return err
		}

		txn := rootHelper.CreateTransaction(ownerKey.Address(), &supernetAddr, encoded, nil, true)

		if _, err = txRelayer.Call(ownerKey.Address(), supernetAddr, encoded); err == nil {
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

		chainConfig, err := chain.ImportFromFile(params.genesisPath)
		if err != nil {
			return fmt.Errorf("failed to read chain configuration: %w", err)
		}

		consensusConfig, err := polybft.GetPolyBFTConfig(chainConfig)
		if err != nil {
			return fmt.Errorf("failed to retrieve consensus configuration: %w", err)
		}

		validatorMetadata := make([]*validator.ValidatorMetadata, len(consensusConfig.InitialValidatorSet))

		genesisSetInput, err := genesisSetABIFn.Encode([]interface{}{})
		if err != nil {
			return fmt.Errorf("failed to encode genesis set input: %w", err)
		}

		genesisSetHexOut, err := txRelayer.Call(ethgo.ZeroAddress, supernetAddr, genesisSetInput)
		if err != nil {
			return fmt.Errorf("failed to retrieve genesis set: %w", err)
		}

		genesisAccounts, err := decodeGenesisAccounts(genesisSetHexOut)
		if err != nil {
			return err
		}

		genesisAccsMap := make(map[types.Address]*validator.GenesisValidator, len(genesisAccounts))

		for _, genesisAcc := range genesisAccounts {
			genesisBalanceInput, err := genesisBalancesABIFn.Encode([]interface{}{genesisAcc.Address})
			if err != nil {
				return err
			}

			genesisBalanceRaw, err := txRelayer.Call(ethgo.ZeroAddress, supernetAddr, genesisBalanceInput)
			if err != nil {
				return err
			}

			genesisBalance, err := common.ParseUint256orHex(&genesisBalanceRaw)
			if err != nil {
				return fmt.Errorf("failed to convert genesis balance '%s' to number: %w",
					genesisBalanceRaw, err)
			}

			genesisAccsMap[genesisAcc.Address] = genesisAcc

			if genesisBalance.Sign() > 0 {
				// premine genesis accounts
				chainConfig.Genesis.Alloc[genesisAcc.Address] =
					&chain.GenesisAccount{Balance: genesisBalance}
			}
		}

		// update Stake field of validators in genesis file
		// based on their finalized stake on rootchain
		for i, v := range consensusConfig.InitialValidatorSet {
			genesisAcc, exists := genesisAccsMap[v.Address]
			if !exists {
				return fmt.Errorf("validator with address %s not found among genesis accounts", v.Address)
			}

			v.Stake = genesisAcc.Stake

			metadata, err := v.ToValidatorMetadata()
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
	}

	if params.enableStaking {
		encoded, err := contractsapi.CustomSupernetManager.Abi.Methods["enableStaking"].Encode([]interface{}{})
		if err != nil {
			return err
		}

		txn := rootHelper.CreateTransaction(ownerKey.Address(), &supernetAddr, encoded, nil, true)

		receipt, err := txRelayer.SendTransaction(txn, ownerKey)
		if err != nil {
			return fmt.Errorf("enabling staking on supernet manager failed. Error: %w", err)
		}

		if receipt.Status == uint64(types.ReceiptFailed) {
			return fmt.Errorf("enable staking transaction failed on block %d", receipt.BlockNumber)
		}
	}

	result := &supernetResult{
		IsGenesisSetFinalized: params.finalizeGenesisSet,
		IsStakingEnabled:      params.enableStaking,
	}

	outputter.WriteCommandResult(result)

	return nil
}

// decodeGenesisAccounts decodes genesis set retrieved from CustomSupernetManager contract
func decodeGenesisAccounts(genesisSetRaw string) ([]*validator.GenesisValidator, error) {
	decodeAccount := func(rawAccount map[string]interface{}) (*validator.GenesisValidator, error) {
		addr, ok := rawAccount["addr"].(ethgo.Address)
		if !ok {
			return nil, errors.New("failed to retrieve genesis account address")
		}

		stake, ok := rawAccount["initialStake"].(*big.Int)
		if !ok {
			return nil, errors.New("failed to retrieve genesis account stake")
		}

		return &validator.GenesisValidator{
			Address: types.Address(addr),
			Stake:   stake,
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

	genesisAccounts := make([]*validator.GenesisValidator, len(decodedGenesisSetSliceMap))
	for i, rawGenesisAccount := range decodedGenesisSetSliceMap {
		genesisAccounts[i], err = decodeAccount(rawGenesisAccount)
		if err != nil {
			return nil, err
		}
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
	deployerKey ethgo.Key) error {
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

	if _, err := rootHelper.SendTransaction(txRelayer, ethgo.Address(consensusConfig.Bridge.CheckpointManagerAddr),
		input, "CheckpointManager", deployerKey); err != nil {
		return err
	}

	outputter.WriteCommandResult(
		&rootHelper.MessageResult{
			Message: fmt.Sprintf("CheckpointManager contract is initialized"),
		})

	return nil
}
