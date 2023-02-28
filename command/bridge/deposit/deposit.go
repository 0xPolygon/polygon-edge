package deposit

import (
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	// defaultMintValue represents amount of tokens which are going to be minted to depositor
	defaultMintValue = int64(1e18)
)

var (
	// params is abstraction for provided bridge parameter values
	params *common.BridgeParams

	// manifest represents static configuration
	manifest *polybft.Manifest

	// configs holds mapping between token type and bridge parameterizable values
	configs = map[common.TokenType]*bridgeConfig{}
)

// GetCommand returns the bridge deposit command
func GetCommand() *cobra.Command {
	depositCmd := &cobra.Command{
		Use:     "deposit",
		Short:   "Deposits tokens from the root chain to the child chain",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	return depositCmd
}

func runPreRun(_ *cobra.Command, _ []string) error {
	var err error

	if err = params.ValidateFlags(); err != nil {
		return err
	}

	manifest, err = polybft.LoadManifest(params.ManifestPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest file from '%s': %w", params.ManifestPath, err)
	}

	// populate bridge configs based on token types
	configs[common.ERC20] = newBridgeConfig(
		contractsapi.RootERC20Predicate,
		"depositTo",
		contractsapi.RootERC20,
		"mint",
		manifest.RootchainConfig.RootNativeERC20Address,
		manifest.RootchainConfig.RootERC20PredicateAddress,
		contracts.ChildERC20PredicateContract)

	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := helper.InitRootchainPrivateKey(params.TxnSenderKey); err != nil {
		outputter.SetError(err)

		return
	}

	tokenType, _ := common.LookupTokenType(params.TokenTypeRaw)

	config, exists := configs[tokenType]
	if !exists {
		outputter.SetError(fmt.Errorf("not found bridge config for provided token type: %s", params.TokenTypeRaw))

		return
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(params.JSONRPCAddress))
	if err != nil {
		outputter.SetError(fmt.Errorf("could not create rootchain tx relayer: %w", err))

		return
	}

	g, ctx := errgroup.WithContext(cmd.Context())

	for i := range params.Receivers {
		receiver := params.Receivers[i]
		amount := params.Amounts[i]

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if helper.IsTestMode(params.TxnSenderKey) {
					// mint tokens to depositor
					txn, err := createMintTxn(config, big.NewInt(defaultMintValue))
					if err != nil {
						return fmt.Errorf("mint transaction creation failed: %w", err)
					}
					receipt, err := txRelayer.SendTransaction(txn, helper.GetRootchainPrivateKey())
					if err != nil {
						return fmt.Errorf("failed to send mint transaction to depositor %s", helper.GetRootchainPrivateKey().Address())
					}

					if receipt.Status == uint64(types.ReceiptFailed) {
						return fmt.Errorf("failed to mint tokens to depositor %s", helper.GetRootchainPrivateKey().Address())
					}
				}

				// deposit tokens
				amountBig, err := types.ParseUint256orHex(&amount)
				if err != nil {
					return fmt.Errorf("failed to decode provided amount %s: %w", amount, err)
				}
				txn, err := createDepositTxn(config, ethgo.BytesToAddress([]byte(receiver)), amountBig)
				if err != nil {
					return fmt.Errorf("failed to create tx input: %w", err)
				}

				receipt, err := txRelayer.SendTransaction(txn, helper.GetRootchainPrivateKey())
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
		outputter.SetError(fmt.Errorf("sending deposit transactions to the rootchain failed: %w", err))

		return
	}

	outputter.SetCommandResult(&result{
		TokenType: params.TokenTypeRaw,
		Sender:    helper.GetRootchainPrivateKey().Address().String(),
		Receivers: params.Receivers,
		Amounts:   params.Amounts,
	})
}

// createDepositTxn encodes parameters for deposit function on rootchain predicate contract
func createDepositTxn(config *bridgeConfig, receiver ethgo.Address, amount *big.Int) (*ethgo.Transaction, error) {
	input, err := config.rootPredicate.Abi.Methods[config.depositFnName].Encode([]interface{}{
		config.rootTokenAddr,
		receiver,
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
		helper.GetRootchainPrivateKey().Address(),
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
