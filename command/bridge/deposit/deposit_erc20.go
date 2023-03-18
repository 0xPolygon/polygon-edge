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
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	rootTokenFlag     = "root-token"
	rootPredicateFlag = "root-predicate"
	jsonRPCFlag       = "json-rpc"
)

type depositERC20Params struct {
	*common.ERC20BridgeParams
	rootTokenAddr     string
	rootPredicateAddr string
	jsonRPCAddress    string
	testMode          bool
}

var (
	// depositParams is abstraction for provided bridge parameter values
	dp *depositERC20Params = &depositERC20Params{ERC20BridgeParams: &common.ERC20BridgeParams{}}
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
		&dp.AccountDir,
		polybftsecrets.AccountDirFlag,
		"",
		polybftsecrets.AccountDirFlagDesc,
	)

	depositCmd.Flags().StringVar(
		&dp.AccountConfig,
		polybftsecrets.AccountConfigFlag,
		"",
		polybftsecrets.AccountConfigFlagDesc,
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
		"root ERC20 token address",
	)

	depositCmd.Flags().StringVar(
		&dp.rootPredicateAddr,
		rootPredicateFlag,
		"",
		"root ERC20 token predicate address",
	)

	depositCmd.Flags().StringVar(
		&dp.jsonRPCAddress,
		jsonRPCFlag,
		"http://127.0.0.1:8545",
		"the JSON RPC root chain endpoint",
	)

	depositCmd.Flags().BoolVar(
		&dp.testMode,
		helper.TestModeFlag,
		false,
		"test indicates whether depositor is hardcoded test account "+
			"(in that case tokens are minted to it, so it is able to make deposits)",
	)

	depositCmd.MarkFlagRequired(common.ReceiversFlag) //nolint:errcheck
	depositCmd.MarkFlagRequired(common.AmountsFlag)   //nolint:errcheck
	depositCmd.MarkFlagRequired(rootTokenFlag)        //nolint:errcheck
	depositCmd.MarkFlagRequired(rootPredicateFlag)    //nolint:errcheck

	depositCmd.MarkFlagsMutuallyExclusive(
		helper.TestModeFlag,
		polybftsecrets.AccountDirFlag,
		polybftsecrets.AccountConfigFlag)

	return depositCmd
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	if err := dp.ValidateFlags(dp.testMode); err != nil {
		return err
	}

	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	var depositorKey ethgo.Key

	if !dp.testMode {
		depositorAccount, err := sidechain.GetAccount(dp.AccountDir, dp.AccountConfig)
		if err != nil {
			outputter.SetError(err)

			return
		}

		depositorKey = depositorAccount.Ecdsa
	} else {
		rootchainKey, err := helper.GetRootchainTestPrivKey()
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to initialize root chain private key: %w", err))

			return
		}

		depositorKey = rootchainKey
	}

	depositorAddr := depositorKey.Address()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(dp.jsonRPCAddress))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize rootchain tx relayer: %w", err))

		return
	}

	amounts := make([]*big.Int, len(dp.Amounts))
	aggregateAmount := new(big.Int)

	for i, amountRaw := range dp.Amounts {
		amountRaw := amountRaw

		amount, err := types.ParseUint256orHex(&amountRaw)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to decode provided amount %s: %w", amount, err))

			return
		}

		amounts[i] = amount
		aggregateAmount.Add(aggregateAmount, amount)
	}

	if dp.testMode {
		// mint tokens to depositor, so he is able to send them
		mintTxn, err := createMintTxn(types.Address(depositorAddr), types.Address(depositorAddr), aggregateAmount)
		if err != nil {
			outputter.SetError(fmt.Errorf("mint transaction creation failed: %w", err))

			return
		}

		receipt, err := txRelayer.SendTransaction(mintTxn, depositorKey)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to send mint transaction to depositor %s", depositorAddr))

			return
		}

		if receipt.Status == uint64(types.ReceiptFailed) {
			outputter.SetError(fmt.Errorf("failed to mint tokens to depositor %s", depositorAddr))

			return
		}
	}

	// approve root erc20 predicate
	approveTxn, err := createApproveERC20PredicateTxn(aggregateAmount,
		types.StringToAddress(dp.rootPredicateAddr),
		types.StringToAddress(dp.rootTokenAddr))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create root erc20 approve transaction: %w", err))

		return
	}

	receipt, err := txRelayer.SendTransaction(approveTxn, depositorKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to send root erc20 approve transaction"))

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		outputter.SetError(fmt.Errorf("failed to approve root erc20 predicate"))

		return
	}

	g, ctx := errgroup.WithContext(cmd.Context())

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

				receipt, err = txRelayer.SendTransaction(depositTxn, depositorKey)
				if err != nil {
					return fmt.Errorf("receiver: %s, amount: %s, error: %w", receiver, amount, err)
				}

				if receipt.Status == uint64(types.ReceiptFailed) {
					return fmt.Errorf("receiver: %s, amount: %s", receiver, amount)
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
		Sender:    depositorAddr.String(),
		Receivers: dp.Receivers,
		Amounts:   dp.Amounts,
	})
}

// createDepositTxn encodes parameters for deposit function on rootchain predicate contract
func createDepositTxn(sender, receiver types.Address, amount *big.Int) (*ethgo.Transaction, error) {
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
		From:  ethgo.Address(sender),
		To:    &addr,
		Input: input,
	}, nil
}

// createMintTxn encodes parameters for mint function on rootchain token contract
func createMintTxn(sender, receiver types.Address, amount *big.Int) (*ethgo.Transaction, error) {
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
		From:  ethgo.Address(sender),
		To:    &addr,
		Input: input,
	}, nil
}

// createApproveERC20PredicateTxn sends approve transaction
// to ERC20 token for ERC20 predicate so that it is able to spend given tokens
func createApproveERC20PredicateTxn(amount *big.Int,
	rootERC20Predicate, rootERC20Token types.Address) (*ethgo.Transaction, error) {
	approveFnParams := &contractsapi.ApproveFunction{
		Spender: rootERC20Predicate,
		Amount:  amount,
	}

	input, err := approveFnParams.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode parameters for RootERC20.approve. error: %w", err)
	}

	addr := ethgo.Address(rootERC20Token)

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
