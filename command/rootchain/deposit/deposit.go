package deposit

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

type TokenType int

const (
	ERC20   TokenType = iota // ERC20   = 0
	ERC721                   // ERC721 	= 1
	ERC1155                  // ERC1155 = 2
)

const (
	// defaultMintValue represents amount of tokens which are going to be minted to depositor
	defaultMintValue = 1000000000000000
)

var (
	params depositParams

	manifest *polybft.Manifest

	tokenTypesMap = map[string]TokenType{
		"erc20":   ERC20,
		"erc721":  ERC721,
		"erc1155": ERC1155,
	}

	configs = map[TokenType]*bridgeConfig{}
)

// GetCommand returns the rootchain deposit command
func GetCommand() *cobra.Command {
	depositCmd := &cobra.Command{
		Use:     "deposit",
		Short:   "Deposits tokens from root chain to child chain",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(depositCmd)

	return depositCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.manifestPath,
		manifestPathFlag,
		"./manifest.json",
		"the manifest file path, which contains genesis metadata",
	)

	cmd.Flags().StringVar(
		&params.tokenTypeRaw,
		tokenFlag,
		"erc20",
		"Token type which is being deposited",
	)

	cmd.Flags().StringSliceVar(
		&params.receivers,
		receiversFlag,
		nil,
		"list of wallet addresses",
	)

	cmd.Flags().StringSliceVar(
		&params.amounts,
		amountsFlag,
		nil,
		"list of amounts to fund wallets",
	)

	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCFlag,
		"http://127.0.0.1:8545",
		"the JSON RPC rootchain IP address (e.g. http://127.0.0.1:8545)",
	)

	cmd.Flags().StringVar(
		&params.adminKey,
		adminKeyFlag,
		helper.DefaultPrivateKeyRaw,
		"Hex encoded private key of the account which sends rootchain transactions",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	var err error
	if err = params.validateFlags(); err != nil {
		return err
	}

	manifest, err = polybft.LoadManifest(params.manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest file from '%s': %w", params.manifestPath, err)
	}

	// populate bridge configs based on token types
	configs[ERC20] = newBridgeConfig(
		contractsapi.RootERC20Predicate,
		"depositTo",
		contractsapi.MockERC20,
		"mint",
		manifest.RootchainConfig.RootERC20Address,
		manifest.RootchainConfig.RootERC20PredicateAddress,
		contracts.ChildERC20PredicateContract)

	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	err := helper.InitRootchainAdminKey(params.adminKey)
	if err != nil {
		outputter.SetError(err)

		return
	}

	tokenType, _ := lookupTokenType(params.tokenTypeRaw)

	config, exists := configs[tokenType]
	if !exists {
		outputter.SetError(fmt.Errorf("not found bridge config for provided token type: %s", params.tokenTypeRaw))

		return
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.jsonRPCAddress))
	if err != nil {
		outputter.SetError(fmt.Errorf("could not create rootchain interactor: %w", err))

		return
	}

	g, ctx := errgroup.WithContext(cmd.Context())

	for i := range params.receivers {
		receiver := params.receivers[i]
		amount := params.amounts[i]

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				// mint tokens to transaction sender
				txn, err := createMintTxn(config, big.NewInt(defaultMintValue))
				if err != nil {
					return fmt.Errorf("mint transaction creation failed: %w", err)
				}
				receipt, err := txRelayer.SendTransaction(txn, helper.GetRootchainAdminKey())
				if err != nil {
					return fmt.Errorf("mint transaction failed")
				}

				// deposit tokens
				amountBig, err := types.ParseUint256orHex(&amount)
				if err != nil {
					return fmt.Errorf("failed to decode provided amount %s: %w", amount, err)
				}
				txn, err = createDepositTxn(config, ethgo.BytesToAddress([]byte(receiver)), amountBig)
				if err != nil {
					return fmt.Errorf("failed to create tx input: %w", err)
				}

				receipt, err = txRelayer.SendTransaction(txn, helper.GetRootchainAdminKey())
				if err != nil {
					return fmt.Errorf("receiver: %s, amount: %s, error: %w",
						receiver, amount, err)
				}

				if receipt.Status == uint64(types.ReceiptFailed) {
					return fmt.Errorf("receiver: %s, amount: %s",
						receiver, amount)
				}

				return nil
			}
		})
	}

	if err = g.Wait(); err != nil {
		outputter.SetError(fmt.Errorf("sending transactions to rootchain failed: %w", err))

		return
	}

	outputter.SetCommandResult(&result{
		TokenType: params.tokenTypeRaw,
		Receivers: params.receivers,
		Amounts:   params.amounts,
	})
}

// createDepositTxn encodes parameters for deposit function on rootchain predicate contract
func createDepositTxn(config *bridgeConfig, receiver ethgo.Address, amount *big.Int) (*ethgo.Transaction, error) {
	input, err := config.rootPredicate.Abi.Methods[config.depositFnName].Encode([]interface{}{
		config.rootTokenAddr,
		config.childPredicateAddr,
		amount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(config.rootPredicateAddr)

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}

// createMintTxn encodes parameters for mint function on rootchain token contract
func createMintTxn(config *bridgeConfig, amount *big.Int) (*ethgo.Transaction, error) {
	input, err := config.rootToken.Abi.Methods[config.mintFnName].Encode([]interface{}{
		helper.GetRootchainAdminKey().Address(),
		amount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(config.rootTokenAddr)

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}

// bridgeConfig contains parameterizable parameters for assets bridging
type bridgeConfig struct {
	rootPredicate      *artifact.Artifact
	depositFnName      string
	rootToken          *artifact.Artifact
	mintFnName         string
	rootTokenAddr      types.Address
	rootPredicateAddr  types.Address
	childPredicateAddr types.Address
}

func newBridgeConfig(rootPredicate *artifact.Artifact,
	depositFnName string,
	rootToken *artifact.Artifact,
	mintFnName string,
	rootTokenAddr types.Address,
	rootPredicateAddr types.Address,
	childPredicateAddr types.Address) *bridgeConfig {
	return &bridgeConfig{
		rootPredicate:      rootPredicate,
		depositFnName:      depositFnName,
		rootToken:          rootToken,
		mintFnName:         mintFnName,
		rootTokenAddr:      rootTokenAddr,
		rootPredicateAddr:  rootPredicateAddr,
		childPredicateAddr: childPredicateAddr,
	}
}

// lookupTokenType looks up for provided token type string and returns resolved enum value if found
func lookupTokenType(tokenTypeRaw string) (TokenType, bool) {
	tokenType, ok := tokenTypesMap[strings.ToLower(tokenTypeRaw)]

	return tokenType, ok
}
