package exit

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/command/bridge/helper"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	// flag names
	exitHelperFlag   = "exit-helper"
	exitEventIDFlag  = "exit-id"
	rootJSONRPCFlag  = "root-json-rpc"
	childJSONRPCFlag = "child-json-rpc"

	// generateExitProofFn is JSON RPC endpoint which creates exit proof
	generateExitProofFn = "bridge_generateExitProof"
)

type exitParams struct {
	senderKey         string
	exitHelperAddrRaw string
	exitID            uint64
	rootJSONRPCAddr   string
	childJSONRPCAddr  string
	txTimeout         time.Duration
}

var (
	// ep represents exit command parameters
	ep *exitParams = &exitParams{}
)

// GetCommand returns the bridge exit command
func GetCommand() *cobra.Command {
	exitCmd := &cobra.Command{
		Use:   "exit",
		Short: "Sends exit transaction to the Exit helper contract on the root chain",
		Run:   run,
	}

	exitCmd.Flags().StringVar(
		&ep.senderKey,
		common.SenderKeyFlag,
		"",
		"hex encoded private key of the account which sends exit transaction to the root chain",
	)

	exitCmd.Flags().StringVar(
		&ep.exitHelperAddrRaw,
		exitHelperFlag,
		"",
		"address of ExitHelper smart contract on root chain",
	)

	exitCmd.Flags().Uint64Var(
		&ep.exitID,
		exitEventIDFlag,
		0,
		"child chain exit event ID",
	)

	exitCmd.Flags().StringVar(
		&ep.rootJSONRPCAddr,
		rootJSONRPCFlag,
		txrelayer.DefaultRPCAddress,
		"the JSON RPC root chain endpoint",
	)

	exitCmd.Flags().StringVar(
		&ep.childJSONRPCAddr,
		childJSONRPCFlag,
		"http://127.0.0.1:9545",
		"the JSON RPC child chain endpoint",
	)

	exitCmd.Flags().DurationVar(
		&ep.txTimeout,
		cmdHelper.TxTimeoutFlag,
		txrelayer.DefaultTimeoutTransactions,
		cmdHelper.TxTimeoutDesc,
	)

	_ = exitCmd.MarkFlagRequired(exitHelperFlag)

	return exitCmd
}

func run(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	senderKey, err := helper.DecodePrivateKey(ep.senderKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create wallet from private key: %w", err))

		return
	}

	rootTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(ep.rootJSONRPCAddr),
		txrelayer.WithReceiptsTimeout(ep.txTimeout))
	if err != nil {
		outputter.SetError(fmt.Errorf("could not create root chain tx relayer: %w", err))

		return
	}

	childClient, err := jsonrpc.NewClient(ep.childJSONRPCAddr)
	if err != nil {
		outputter.SetError(fmt.Errorf("could not create child chain JSON RPC client: %w", err))

		return
	}

	// acquire proof for given exit event
	var proof types.Proof

	err = childClient.Call(generateExitProofFn, &proof, fmt.Sprintf("0x%x", ep.exitID))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to get exit proof (exit id=%d): %w", ep.exitID, err))

		return
	}

	// create exit transaction
	txn, exitEvent, err := createExitTxn(senderKey.Address(), proof)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create tx input: %w", err))

		return
	}

	// send exit transaction
	receipt, err := rootTxRelayer.SendTransaction(txn, senderKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to send exit transaction (exit id=%d): %w", ep.exitID, err))

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		outputter.SetError(fmt.Errorf("failed to execute exit transaction (exit id=%d)", ep.exitID))

		return
	}

	outputter.SetCommandResult(&exitResult{
		ID:       strconv.FormatUint(exitEvent.ID.Uint64(), 10),
		Sender:   exitEvent.Sender.String(),
		Receiver: exitEvent.Receiver.String(),
	})
}

// createExitTxn encodes parameters for exit function on root chain ExitHelper contract
func createExitTxn(sender types.Address, proof types.Proof) (*types.Transaction,
	*contractsapi.L2StateSyncedEvent, error) {
	exitInput, err := polybft.GetExitInputFromProof(proof)
	if err != nil {
		return nil, nil, err
	}

	exitEvent := new(contractsapi.L2StateSyncedEvent)
	if err := exitEvent.Decode(exitInput.UnhashedLeaf); err != nil {
		return nil, nil, fmt.Errorf("failed to decode exit event: %w", err)
	}

	exitFn := &contractsapi.ExitExitHelperFn{
		BlockNumber:  exitInput.BlockNumber,
		LeafIndex:    exitInput.LeafIndex,
		UnhashedLeaf: exitInput.UnhashedLeaf,
		Proof:        exitInput.Proof,
	}

	input, err := exitFn.EncodeAbi()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	exitHelperAddr := types.StringToAddress(ep.exitHelperAddrRaw)
	txn := helper.CreateTransaction(sender, &exitHelperAddr, input, nil, true)
	txn.SetGas(txrelayer.DefaultGasLimit)

	return txn, exitEvent, err
}

type exitResult struct {
	ID       string `json:"id"`
	Sender   string `json:"sender"`
	Receiver string `json:"receiver"`
}

func (r *exitResult) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Exit Event ID|%s", r.ID))
	vals = append(vals, fmt.Sprintf("Sender|%s", r.Sender))
	vals = append(vals, fmt.Sprintf("Receiver|%s", r.Receiver))

	buffer.WriteString("\n[EXIT TRANSACTION RELAYER]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
