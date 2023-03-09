package withdraw

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	// flag names
	exitHelperFlag      = "exit-helper"
	exitEventIDFlag     = "event-id"
	epochFlag           = "epoch"
	checkpointBlockFlag = "checkpoint-block"
	rootJSONRPCFlag     = "root-json-rpc"
	childJSONRPCFlag    = "child-json-rpc"

	// generateExitProofFn is JSON RPC endpoint which creates exit proof
	generateExitProofFn = "bridge_generateExitProof"
)

type exitParams struct {
	txnSenderKey      string
	exitHelperAddrRaw string
	exitID            uint64
	epochNumber       uint64
	checkpointBlock   uint64
	rootJSONRPCAddr   string
	childJSONRPCAddr  string
}

var (
	// ep represents exit command parameters
	ep *exitParams = &exitParams{}
)

// GetExitCommand returns the bridge exit command
func GetExitCommand() *cobra.Command {
	exitCmd := &cobra.Command{
		Use:   "exit",
		Short: "Performs exit transaction from the child chain to the root chain",
		Run:   runExitCommand,
	}

	exitCmd.Flags().StringVar(
		&ep.txnSenderKey,
		common.SenderKeyFlag,
		helper.DefaultPrivateKeyRaw,
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

	exitCmd.Flags().Uint64Var(
		&ep.epochNumber,
		epochFlag,
		0,
		"child chain exit event epoch number",
	)

	exitCmd.Flags().Uint64Var(
		&ep.checkpointBlock,
		checkpointBlockFlag,
		0,
		"child chain exit event checkpoint block",
	)

	exitCmd.Flags().StringVar(
		&ep.rootJSONRPCAddr,
		rootJSONRPCFlag,
		"http://127.0.0.1:8545",
		"the JSON RPC root chain endpoint",
	)

	exitCmd.Flags().StringVar(
		&ep.childJSONRPCAddr,
		childJSONRPCFlag,
		"http://127.0.0.1:9545",
		"the JSON RPC child chain endpoint",
	)

	exitCmd.MarkFlagRequired(exitHelperFlag)

	return exitCmd
}

func runExitCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ecdsaRaw, err := hex.DecodeString(ep.txnSenderKey)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to decode private key: %w", err))

		return
	}

	key, err := wallet.NewWalletFromPrivKey(ecdsaRaw)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create wallet from private key: %w", err))

		return
	}

	rootTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(ep.rootJSONRPCAddr))
	if err != nil {
		outputter.SetError(fmt.Errorf("could not create root chain tx relayer: %w", err))

		return
	}

	childClient, err := jsonrpc.NewClient(ep.childJSONRPCAddr)
	if err != nil {
		outputter.SetError(fmt.Errorf("could not create child chain JSON RPC client: %w", err))

		return
	}

	var proof types.Proof

	err = childClient.Call(generateExitProofFn, &proof,
		fmt.Sprintf("0x%x", ep.exitID),
		fmt.Sprintf("0x%x", ep.epochNumber),
		fmt.Sprintf("0x%x", ep.checkpointBlock))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to get exit proof (exit id=%d, epoch number=%d, checkpoint block=%d): %w",
			ep.exitID, ep.epochNumber, ep.checkpointBlock, err))

		return
	}

	// exit transaction
	txn, exitEvent, err := createExitTxn(proof, outputter)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create tx input: %w", err))

		return
	}

	receipt, err := rootTxRelayer.SendTransaction(txn, key)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to send exit transaction "+
			"(exit id=%d, epoch number=%d, checkpoint block=%d): %w",
			ep.exitID, ep.epochNumber, ep.checkpointBlock, err))

		return
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		outputter.SetError(fmt.Errorf("failed to execute exit transaction (exit id=%d, epoch number=%d, checkpoint block=%d)",
			ep.exitID, ep.epochNumber, ep.checkpointBlock))

		return
	}

	outputter.SetCommandResult(&exitResult{
		ID:       strconv.FormatUint(exitEvent.ID, 10),
		Sender:   exitEvent.Sender.String(),
		Receiver: exitEvent.Receiver.String(),
	})
}

// createExitTxn encodes parameters for exit function on root chain ExitHelper contract
func createExitTxn(proof types.Proof, output command.OutputFormatter) (*ethgo.Transaction, *polybft.ExitEvent, error) {
	exitEventMap, ok := proof.Metadata["ExitEvent"].(map[string]interface{})
	if !ok {
		return nil, nil, errors.New("could not get exit event from proof")
	}

	raw, err := json.Marshal(exitEventMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal exit event map to JSON. Error: %w", err)
	}

	var exitEvent *polybft.ExitEvent
	if err = json.Unmarshal(raw, &exitEvent); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal exit event from JSON. Error: %w", err)
	}

	exitEventEncoded, err := polybft.ExitEventInputsABIType.Encode(exitEvent)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode exit event: %w", err)
	}

	leafIndex, ok := proof.Metadata["LeafIndex"].(float64)
	if !ok {
		return nil, nil, errors.New("failed to convert proof leaf index to float64")
	}

	exitFn := &contractsapi.ExitFunction{
		BlockNumber:  new(big.Int).SetUint64(ep.checkpointBlock),
		LeafIndex:    new(big.Int).SetUint64(uint64(leafIndex)),
		UnhashedLeaf: exitEventEncoded,
		Proof:        proof.Data,
	}

	input, err := exitFn.EncodeAbi()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	exitHelperAddr := ethgo.Address(types.StringToAddress(ep.exitHelperAddrRaw))
	txn := &ethgo.Transaction{
		To:    &exitHelperAddr,
		Input: input,
	}

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

	buffer.WriteString("\n[EXIT HELPER]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
