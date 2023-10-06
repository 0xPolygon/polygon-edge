package erc20

import (
	"fmt"
	"math/big"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	helperCommon "github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

type depositERC20Params struct {
	*common.ERC20BridgeParams
	minterKey string
}

var (
	// depositParams is abstraction for provided bridge parameter values
	dp *depositERC20Params = &depositERC20Params{ERC20BridgeParams: common.NewERC20BridgeParams()}
)

// GetCommand returns the bridge deposit command
func GetCommand() *cobra.Command {
	depositCmd := &cobra.Command{
		Use:     "deposit-erc20",
		Short:   "Deposits ERC 20 tokens from the origin to the destination chain",
		PreRunE: preRunCommand,
		Run:     runCommand,
	}

	dp.RegisterCommonFlags(depositCmd)

	depositCmd.Flags().StringSliceVar(
		&dp.Amounts,
		common.AmountsFlag,
		nil,
		"amounts to send to receiving accounts",
	)

	depositCmd.Flags().StringVar(
		&dp.TokenAddr,
		common.RootTokenFlag,
		"",
		"root ERC 20 token address",
	)

	depositCmd.Flags().StringVar(
		&dp.PredicateAddr,
		common.RootPredicateFlag,
		"",
		"root ERC 20 token predicate address",
	)

	depositCmd.Flags().StringVar(
		&dp.minterKey,
		common.MinterKeyFlag,
		"",
		common.MinterKeyFlagDesc,
	)

	_ = depositCmd.MarkFlagRequired(common.ReceiversFlag)
	_ = depositCmd.MarkFlagRequired(common.AmountsFlag)
	_ = depositCmd.MarkFlagRequired(common.RootTokenFlag)
	_ = depositCmd.MarkFlagRequired(common.RootPredicateFlag)

	return depositCmd
}

func preRunCommand(cmd *cobra.Command, _ []string) error {
	return dp.Validate()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	depositorKey, err := helper.DecodePrivateKey(dp.SenderKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize depositor private key: %w", err))
	}

	depositorAddr := depositorKey.Address()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(dp.JSONRPCAddr))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize tx relayer: %w", err))

		return
	}

	amounts := make([]*big.Int, len(dp.Amounts))
	aggregateAmount := new(big.Int)

	for i, amountRaw := range dp.Amounts {
		amountRaw := amountRaw

		amount, err := helperCommon.ParseUint256orHex(&amountRaw)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to decode provided amount %s: %w", amountRaw, err))

			return
		}

		amounts[i] = amount
		aggregateAmount.Add(aggregateAmount, amount)
	}

	if dp.minterKey != "" {
		minterKey, err := helper.DecodePrivateKey(dp.minterKey)
		if err != nil {
			outputter.SetError(fmt.Errorf("invalid minter key provided: %w", err))

			return
		}

		// mint tokens to depositor, so he is able to send them
		mintTxn, err := helper.CreateMintTxn(types.Address(depositorAddr),
			types.StringToAddress(dp.TokenAddr), aggregateAmount, !dp.ChildChainMintable)
		if err != nil {
			outputter.SetError(fmt.Errorf("mint transaction creation failed: %w", err))

			return
		}

		receipt, err := txRelayer.SendTransaction(mintTxn, minterKey)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to send mint transaction to depositor %s: %w", depositorAddr, err))

			return
		}

		if receipt.Status == uint64(types.ReceiptFailed) {
			outputter.SetError(fmt.Errorf("failed to mint tokens to depositor %s", depositorAddr))

			return
		}
	}

	// approve erc20 predicate
	approveTxn, err := helper.CreateApproveERC20Txn(aggregateAmount,
		types.StringToAddress(dp.PredicateAddr),
		types.StringToAddress(dp.TokenAddr),
		!dp.ChildChainMintable,
	)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create root erc 20 approve transaction: %w", err))

		return
	}

	receipt, err := txRelayer.SendTransaction(approveTxn, depositorKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to send root erc 20 approve transaction: %w", err))

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		outputter.SetError(fmt.Errorf("failed to approve root erc 20 predicate"))

		return
	}

	type bridgeTxData struct {
		exitEventIDs   []*big.Int
		blockNumber    uint64
		childTokenAddr *types.Address
	}

	g, ctx := errgroup.WithContext(cmd.Context())
	bridgeTxCh := make(chan bridgeTxData, len(dp.Receivers))
	exitEventIDs := make([]*big.Int, 0, len(dp.Receivers))
	blockNumbers := make([]uint64, 0, len(dp.Receivers))

	for i := range dp.Receivers {
		receiver := dp.Receivers[i]
		amount := amounts[i]

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				// deposit tokens
				depositTxn, err := createDepositTxn(types.Address(depositorAddr), types.StringToAddress(receiver), amount)
				if err != nil {
					return fmt.Errorf("failed to create tx input: %w", err)
				}

				var receipt *ethgo.Receipt

				receipt, err = txRelayer.SendTransaction(depositTxn, depositorKey)
				if err != nil {
					return fmt.Errorf("receiver: %s, amount: %s, error: %w", receiver, amount, err)
				}

				if receipt.Status == uint64(types.ReceiptFailed) {
					return fmt.Errorf("receiver: %s, amount: %s", receiver, amount)
				}

				var exitEventIDs []*big.Int

				if dp.ChildChainMintable {
					if exitEventIDs, err = common.ExtractExitEventIDs(receipt); err != nil {
						return fmt.Errorf("failed to extract exit event: %w", err)
					}
				}

				// populate child token address if a token is mapped alongside with deposit
				childToken, err := common.ExtractChildTokenAddr(receipt, dp.ChildChainMintable)
				if err != nil {
					return fmt.Errorf("failed to extract child token address: %w", err)
				}

				// send aggregated data to channel if everything went ok
				bridgeTxCh <- bridgeTxData{
					blockNumber:    receipt.BlockNumber,
					exitEventIDs:   exitEventIDs,
					childTokenAddr: childToken,
				}

				return nil
			}
		})
	}

	if err = g.Wait(); err != nil {
		outputter.SetError(fmt.Errorf("sending deposit transactions failed: %w", err))

		return
	}

	close(bridgeTxCh)

	var childToken *types.Address

	for x := range bridgeTxCh {
		if x.exitEventIDs != nil {
			exitEventIDs = append(exitEventIDs, x.exitEventIDs...)
		}

		blockNumbers = append(blockNumbers, x.blockNumber)

		if x.childTokenAddr != nil {
			childToken = x.childTokenAddr
		}
	}

	outputter.SetCommandResult(
		&common.BridgeTxResult{
			Sender:         depositorAddr.String(),
			Receivers:      dp.Receivers,
			Amounts:        dp.Amounts,
			ExitEventIDs:   exitEventIDs,
			ChildTokenAddr: childToken,
			BlockNumbers:   blockNumbers,
			Title:          "DEPOSIT ERC 20",
		})
}

// createDepositTxn encodes parameters for deposit function on rootchain predicate contract
func createDepositTxn(sender, receiver types.Address, amount *big.Int) (*ethgo.Transaction, error) {
	depositToFn := &contractsapi.DepositToRootERC20PredicateFn{
		RootToken: types.StringToAddress(dp.TokenAddr),
		Receiver:  receiver,
		Amount:    amount,
	}

	input, err := depositToFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(types.StringToAddress(dp.PredicateAddr))

	return helper.CreateTransaction(ethgo.Address(sender), &addr,
		input, nil, !dp.ChildChainMintable), nil
}
