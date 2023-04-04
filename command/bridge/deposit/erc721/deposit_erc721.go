package deposit

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
)

type depositERC721Params struct {
	*common.ERC721BridgeParams
	testMode bool
}

var (
	dp *depositERC721Params = &depositERC721Params{ERC721BridgeParams: common.NewERC721BridgeParams()}
)

func GetCommand() *cobra.Command {
	depositCmd := &cobra.Command{
		Use:     "deposit-erc721",
		Short:   "Deposits ERC721 tokens from the root chain to the child chain",
		PreRunE: preRunCommand,
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
		&dp.TokenIDs,
		common.TokenIDsFlag,
		nil,
		"token ids that are sent to the receivers accounts",
	)

	depositCmd.Flags().StringVar(
		&dp.TokenAddr,
		common.RootTokenFlag,
		"",
		"root ERC 721 token address",
	)

	depositCmd.Flags().StringVar(
		&dp.PredicateAddr,
		common.RootPredicateFlag,
		"",
		"root ERC 721 token predicate address",
	)

	depositCmd.Flags().StringVar(
		&dp.JSONRPCAddr,
		common.JSONRPCFlag,
		txrelayer.DefaultRPCAddress,
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
	_ = depositCmd.MarkFlagRequired(common.TokenIDsFlag)
	_ = depositCmd.MarkFlagRequired(common.RootTokenFlag)
	_ = depositCmd.MarkFlagRequired(common.RootPredicateFlag)

	depositCmd.MarkFlagsMutuallyExclusive(helper.TestModeFlag, common.SenderKeyFlag)

	return depositCmd
}

func preRunCommand(_ *cobra.Command, _ []string) error {
	return dp.Validate()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	depositorKey, err := helper.GetRootchainPrivateKey(dp.SenderKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize depositor private key: %w", err))
	}

	depositorAddr := depositorKey.Address()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(dp.JSONRPCAddr))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to initialize rootchain tx relayer: %w", err))

		return
	}

	receivers := make([]ethgo.Address, len(dp.Receivers))
	tokenIDs := make([]*big.Int, len(dp.Receivers))

	for i, tokenIDRaw := range dp.TokenIDs {
		tokenIDRaw := tokenIDRaw

		tokenID, err := types.ParseUint256orHex(&tokenIDRaw)
		if err != nil {
			outputter.SetError(fmt.Errorf("failed to decode provided token id %s: %w", tokenIDRaw, err))

			return
		}

		receivers[i] = ethgo.Address(types.StringToAddress(dp.Receivers[i]))
		tokenIDs[i] = tokenID
	}

	if dp.testMode {
		for i := 0; i < len(tokenIDs); i++ {
			mintTxn, err := createMintTxn(types.Address(depositorAddr), types.Address(depositorAddr))
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
	}

	approveTxn, err := createApproveERC721PredicateTxn(types.StringToAddress(dp.PredicateAddr),
		types.StringToAddress(dp.TokenAddr))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to approve predicate: %w", err))

		return
	}

	receipt, err := txRelayer.SendTransaction(approveTxn, depositorKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to send root erc 721 approve transaction"))

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		outputter.SetError(fmt.Errorf("failed to approve root erc 721 predicate"))

		return
	}

	// deposit tokens
	depositTxn, err := createDepositTxn(depositorAddr, receivers, tokenIDs)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create tx input: %w", err))

		return
	}

	receipt, err = txRelayer.SendTransaction(depositTxn, depositorKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("sending deposit transactions failed (receivers: %s, tokenIDs: %s): %w",
			strings.Join(dp.Receivers, ", "), strings.Join(dp.TokenIDs, ", "), err))

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		outputter.SetError(fmt.Errorf("sending deposit transactions failed (receivers: %s, tokenIDs: %s)",
			strings.Join(dp.Receivers, ", "), strings.Join(dp.TokenIDs, ", ")))

		return
	}

	outputter.SetCommandResult(
		&depositERC721Result{
			Sender:    depositorAddr.String(),
			Receivers: dp.Receivers,
			TokenIDs:  dp.TokenIDs,
		})
}

// createDepositTxn encodes parameters for deposit fnction on rootchain predicate contract
func createDepositTxn(sender ethgo.Address,
	receivers []ethgo.Address, tokenIDs []*big.Int) (*ethgo.Transaction, error) {
	depositToRoot := &contractsapi.DepositBatchRootERC721PredicateFn{
		RootToken: types.StringToAddress(dp.TokenAddr),
		Receivers: receivers,
		TokenIDs:  tokenIDs,
	}

	input, err := depositToRoot.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(types.StringToAddress(dp.PredicateAddr))

	return &ethgo.Transaction{
		From:  sender,
		To:    &addr,
		Input: input,
	}, nil
}

// createMintTxn encodes parameters for mint function on rootchain token contract
func createMintTxn(sender, receiver types.Address) (*ethgo.Transaction, error) {
	mintFn := &contractsapi.MintRootERC721Fn{
		To: receiver,
	}

	input, err := mintFn.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(types.StringToAddress(dp.TokenAddr))

	return &ethgo.Transaction{
		From:  ethgo.Address(sender),
		To:    &addr,
		Input: input,
	}, nil
}

// createApproveERC721PredicateTxn sends approve transaction
func createApproveERC721PredicateTxn(rootERC721Predicate, rootERC721Token types.Address) (*ethgo.Transaction, error) {
	approveFnParams := &contractsapi.SetApprovalForAllRootERC721Fn{
		Operator: rootERC721Predicate,
		Approved: true,
	}

	input, err := approveFnParams.EncodeAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to encode parameters for RootERC721.approve. error: %w", err)
	}

	addr := ethgo.Address(rootERC721Token)

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}

type depositERC721Result struct {
	Sender    string   `json:"sender"`
	Receivers []string `json:"receivers"`
	TokenIDs  []string `json:"tokenIDs"`
}

func (r *depositERC721Result) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Sender|%s", r.Sender))
	vals = append(vals, fmt.Sprintf("Receivers|%s", strings.Join(r.Receivers, ", ")))
	vals = append(vals, fmt.Sprintf("TokenIDs|%s", strings.Join(r.TokenIDs, ", ")))

	buffer.WriteString("\n[DEPOSIT ERC 721]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
