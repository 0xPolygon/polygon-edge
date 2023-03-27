package erc1155

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"

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

var (
	errInconsistentTokenIds = errors.New("receivers and token ids must be equal length")
)

type depositERC1155Params struct {
	*common.ERC20BridgeParams
	tokenIDs          []string
	rootTokenAddr     string
	rootPredicateAddr string
	jsonRPCAddress    string
	testMode          bool
}

var (
	// depositParams is abstraction for provided bridge parameter values
	dp *depositERC1155Params = &depositERC1155Params{ERC20BridgeParams: &common.ERC20BridgeParams{}}
)

// GetCommand returns the bridge deposit command
func GetCommand() *cobra.Command {
	depositCmd := &cobra.Command{
		Use:     "deposit-erc1155",
		Short:   "Deposits ERC1155 tokens from the root chain to the child chain",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	depositCmd.Flags().StringVar(
		&dp.SenderKey,
		common.SenderKeyFlag,
		"",
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
		"amounts that are sent to the receivers accounts",
	)

	depositCmd.Flags().StringSliceVar(
		&dp.tokenIDs,
		common.TokenIDsFlag,
		nil,
		"token ids that are sent to the receivers accounts",
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

	_ = depositCmd.MarkFlagRequired(common.ReceiversFlag)
	_ = depositCmd.MarkFlagRequired(common.AmountsFlag)
	_ = depositCmd.MarkFlagRequired(common.TokenIDsFlag)
	_ = depositCmd.MarkFlagRequired(rootTokenFlag)
	_ = depositCmd.MarkFlagRequired(rootPredicateFlag)

	depositCmd.MarkFlagsMutuallyExclusive(helper.TestModeFlag, common.SenderKeyFlag)

	return depositCmd
}

func runPreRun(_ *cobra.Command, _ []string) error {
	if err := dp.ValidateFlags(dp.testMode); err != nil {
		return err
	}

	if len(dp.Receivers) != len(dp.tokenIDs) {
		return errInconsistentTokenIds
	}

	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	depositorKey, err := helper.GetRootchainPrivateKey(dp.SenderKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize depositor private key: %w", err))
	}

	depositorAddr := depositorKey.Address()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(dp.jsonRPCAddress))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize rootchain tx relayer: %w", err))

		return
	}

	amounts := make([]*big.Int, len(dp.Amounts))
	tokenIDs := make([]*big.Int, len(dp.tokenIDs))

	for i := range dp.Receivers {
		amountRaw := dp.Amounts[i]
		tokenIDRaw := dp.tokenIDs[i]

		amount, err := types.ParseUint256orHex(&amountRaw)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to decode provided amount %s: %w", amountRaw, err))

			return
		}

		amounts[i] = amount

		tokenID, err := types.ParseUint256orHex(&tokenIDRaw)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to decode provided token id %s: %w", tokenIDRaw, err))

			return
		}

		tokenIDs[i] = tokenID
	}

	if dp.testMode {
		// mint tokens to depositor, so he is able to send them
		mintTxn, err := createMintTxn(types.Address(depositorAddr), types.Address(depositorAddr), amounts, tokenIDs)
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

	// approve root erc1155 predicate
	approveTxn, err := createApproveERC1155PredicateTxn(types.StringToAddress(dp.rootPredicateAddr),
		types.StringToAddress(dp.rootTokenAddr))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create root erc1155 approve transaction: %w", err))

		return
	}

	receipt, err := txRelayer.SendTransaction(approveTxn, depositorKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to send root erc1155 approve transaction"))

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		outputter.SetError(fmt.Errorf("failed to approve root erc1155 predicate"))

		return
	}

	receivers := make([]ethgo.Address, len(dp.Receivers))
	for i, receiverRaw := range dp.Receivers {
		receivers[i] = ethgo.Address(types.StringToAddress(receiverRaw))
	}

	// deposit tokens
	depositTxn, err := createDepositTxn(depositorAddr, receivers, amounts, tokenIDs)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create tx input: %w", err))

		return
	}

	receipt, err = txRelayer.SendTransaction(depositTxn, depositorKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("sending deposit transactions failed (receivers: %s, amount: %s): %w",
			strings.Join(dp.Receivers, ", "), strings.Join(dp.Amounts, ", "), err))

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		outputter.SetError(fmt.Errorf("sending deposit transactions failed (receivers: %s, amounts: %s)",
			strings.Join(dp.Receivers, ", "), strings.Join(dp.Amounts, ", ")))

		return
	}

	outputter.SetCommandResult(&depositERC1155Result{
		Sender:    depositorAddr.String(),
		Receivers: dp.Receivers,
		Amounts:   dp.Amounts,
		TokenIDs:  dp.tokenIDs,
	})
}

// createDepositTxn encodes parameters for deposit function on rootchain predicate contract
func createDepositTxn(sender ethgo.Address, receivers []ethgo.Address,
	amounts, tokenIDs []*big.Int) (*ethgo.Transaction, error) {
	depositToFn := &contractsapi.DepositBatchRootERC1155PredicateFn{
		RootToken: types.StringToAddress(dp.rootTokenAddr),
		Receivers: receivers,
		Amounts:   amounts,
		TokenIDs:  tokenIDs,
	}

	input, err := depositToFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(types.StringToAddress(dp.rootPredicateAddr))

	return &ethgo.Transaction{
		From:  sender,
		To:    &addr,
		Input: input,
	}, nil
}

// createMintTxn encodes parameters for mint function on rootchain token contract
func createMintTxn(sender, receiver types.Address, amounts, tokenIDs []*big.Int) (*ethgo.Transaction, error) {
	mintFn := &contractsapi.MintBatchRootERC1155Fn{
		To:      receiver,
		Amounts: amounts,
		IDs:     tokenIDs,
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

// createApproveERC1155PredicateTxn sends approve transaction
// to ERC1155 token for ERC1155 predicate so that it is able to spend given tokens
func createApproveERC1155PredicateTxn(rootERC1155Predicate,
	rootERC1155Token types.Address) (*ethgo.Transaction, error) {
	approveFnParams := &contractsapi.SetApprovalForAllRootERC1155Fn{
		Operator: rootERC1155Predicate,
		Approved: true,
	}

	input, err := approveFnParams.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode parameters for RootERC1155.setApprovalForAll. error: %w", err)
	}

	addr := ethgo.Address(rootERC1155Token)

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}

type depositERC1155Result struct {
	Sender    string   `json:"sender"`
	Receivers []string `json:"receivers"`
	Amounts   []string `json:"amounts"`
	TokenIDs  []string `json:"tokenIds"`
}

func (r *depositERC1155Result) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 4)
	vals = append(vals, fmt.Sprintf("Sender|%s", r.Sender))
	vals = append(vals, fmt.Sprintf("Receivers|%s", strings.Join(r.Receivers, ", ")))
	vals = append(vals, fmt.Sprintf("Amounts|%s", strings.Join(r.Amounts, ", ")))
	vals = append(vals, fmt.Sprintf("Token IDs|%s", strings.Join(r.TokenIDs, ", ")))

	buffer.WriteString("\n[DEPOSIT ERC1155]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
