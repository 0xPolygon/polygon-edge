package deposit

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"golang.org/x/sync/errgroup"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	rootTokenFlag     = "root-token"
	rootPredicateFlag = "root-predicate"
	jsonRPCFlag       = "json-rpc"
)

type depositParams struct {
	*common.ERC20BridgeParams
	rootTokenAddr     string
	rootPredicateAddr string
	jsonRPCAddress    string
}

var (
	// depositParams is abstraction for provided bridge parameter values
	dp *depositParams = &depositParams{ERC20BridgeParams: &common.ERC20BridgeParams{}}
)

// GetCommand returns the bridge deposit command
func GetCommand() *cobra.Command {
	depositCmd := &cobra.Command{
		Use:     "deposit-erc20",
		Short:   "Deposits ERC20 tokens from the root chain to the child chain",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	depositCmd.Flags().StringVar(
		&dp.TxnSenderKey,
		common.SenderKeyFlag,
		helper.DefaultPrivateKeyRaw,
		"hex encoded private key of the account which sends rootchain deposit transactions",
	)

	depositCmd.Flags().StringSliceVar(
		&dp.Receivers,
		common.ReceiversFlag,
		nil,
		"receiving accounts addresses on child chain",
	)

	depositCmd.Flags().StringSliceVar(
		&dp.Amounts,
		common.AmountsFlag,
		nil,
		"amounts to send to receiving accounts",
	)

	depositCmd.Flags().StringVar(
		&dp.rootTokenAddr,
		rootTokenFlag,
		"",
		"ERC20 root chain token address",
	)

	depositCmd.Flags().StringVar(
		&dp.rootPredicateAddr,
		rootPredicateFlag,
		"",
		"ERC20 root chain predicate address",
	)

	depositCmd.Flags().StringVar(
		&dp.jsonRPCAddress,
		jsonRPCFlag,
		"http://127.0.0.1:8545",
		"the JSON RPC root chain endpoint",
	)

	depositCmd.MarkFlagRequired(common.ReceiversFlag)
	depositCmd.MarkFlagRequired(common.AmountsFlag)
	depositCmd.MarkFlagRequired(rootTokenFlag)
	depositCmd.MarkFlagRequired(rootPredicateFlag)

	return depositCmd
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	if err := dp.ValidateFlags(); err != nil {
		return err
	}

	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := helper.InitRootchainPrivateKey(dp.TxnSenderKey); err != nil {
		outputter.SetError(err)

		return
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(dp.jsonRPCAddress))
	if err != nil {
		outputter.SetError(fmt.Errorf("could not create rootchain tx relayer: %w", err))

		return
	}

	g, ctx := errgroup.WithContext(cmd.Context())

	for i := range dp.Receivers {
		receiver := dp.Receivers[i]
		amount := dp.Amounts[i]

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				amountBig, err := types.ParseUint256orHex(&amount)
				if err != nil {
					return fmt.Errorf("failed to decode provided amount %s: %w", amount, err)
				}

				if helper.IsTestMode(dp.TxnSenderKey) {
					// mint tokens to depositor, so he is able to send them
					txn, err := createMintTxn(types.Address(helper.GetRootchainPrivateKey().Address()), amountBig)
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
				txn, err := createDepositTxn(types.StringToAddress(receiver), amountBig)
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

	outputter.SetCommandResult(&depositERC20Result{
		Sender:    helper.GetRootchainPrivateKey().Address().String(),
		Receivers: dp.Receivers,
		Amounts:   dp.Amounts,
	})
}

// createDepositTxn encodes parameters for deposit function on rootchain predicate contract
func createDepositTxn(receiver types.Address, amount *big.Int) (*ethgo.Transaction, error) {
	depositToFn := &contractsapi.DepositToFunction{
		RootToken: types.StringToAddress(dp.rootTokenAddr),
		Receiver:  receiver,
		Amount:    amount,
	}

	input, err := depositToFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(types.StringToAddress(dp.rootPredicateAddr))

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}

// createMintTxn encodes parameters for mint function on rootchain token contract
func createMintTxn(receiver types.Address, amount *big.Int) (*ethgo.Transaction, error) {
	mintFn := &contractsapi.MintFunction{
		To:     receiver,
		Amount: amount,
	}

	input, err := mintFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(types.StringToAddress(dp.rootTokenAddr))

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}

type depositERC20Result struct {
	Sender    string   `json:"sender"`
	Receivers []string `json:"receivers"`
	Amounts   []string `json:"amounts"`
}

func (r *depositERC20Result) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Sender|%s", r.Sender))
	vals = append(vals, fmt.Sprintf("Receivers|%s", strings.Join(r.Receivers, ", ")))
	vals = append(vals, fmt.Sprintf("Amounts|%s", strings.Join(r.Amounts, ", ")))

	buffer.WriteString("\n[DEPOSIT ERC20]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
