package withdraw

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/command"
	cmdHelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
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
	secretsDataPath   string
	secretsConfigPath string
	exitHelperAddrRaw string
	exitID            uint64
	epochNumber       uint64
	checkpointBlock   uint64
	rootJSONRPCAddr   string
	childJSONRPCAddr  string
	testMode          bool
}

func (ep *exitParams) validateFlags() error {
	if !ep.testMode {
		if err := sidechain.ValidateSecretFlags(ep.secretsDataPath, ep.secretsConfigPath); err != nil {
			return err
		}
	}

	return nil
}

var (
	// ep represents exit command parameters
	ep *exitParams = &exitParams{}
)

// GetExitCommand returns the bridge exit command
func GetExitCommand() *cobra.Command {
	exitCmd := &cobra.Command{
		Use:     "exit",
		Short:   "Sends exit transaction to the Exit helper contract on the root chain",
		Run:     runExitCommand,
		PreRunE: runExitPreRun,
	}

	exitCmd.Flags().StringVar(
		&ep.secretsDataPath,
		polybftsecrets.DataPathFlag,
		"",
		polybftsecrets.DataPathFlagDesc,
	)

	exitCmd.Flags().StringVar(
		&ep.secretsConfigPath,
		polybftsecrets.ConfigFlag,
		"",
		polybftsecrets.ConfigFlagDesc,
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

	exitCmd.Flags().BoolVar(
		&ep.testMode,
		helper.TestModeFlag,
		false,
		"test indicates whether exit transaction sender is hardcoded test account",
	)

	exitCmd.MarkFlagRequired(exitHelperFlag)
	exitCmd.MarkFlagsMutuallyExclusive(polybftsecrets.DataPathFlag, polybftsecrets.ConfigFlag)
	exitCmd.MarkFlagsMutuallyExclusive(helper.TestModeFlag, polybftsecrets.DataPathFlag)
	exitCmd.MarkFlagsMutuallyExclusive(helper.TestModeFlag, polybftsecrets.ConfigFlag)

	return exitCmd
}

func runExitCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	var senderKey ethgo.Key

	if !ep.testMode {
		secretsManager, err := polybftsecrets.GetSecretsManager(ep.secretsDataPath, ep.secretsConfigPath, true)
		if err != nil {
			outputter.SetError(err)

			return
		}

		senderAccount, err := wallet.NewAccountFromSecret(secretsManager)
		if err != nil {
			outputter.SetError(err)

			return
		}

		senderKey = senderAccount.Ecdsa
	} else {
		if err := helper.InitRootchainPrivateKey(""); err != nil {
			outputter.SetError(fmt.Errorf("failed to initialize root chain private key: %w", err))

			return
		}

		senderKey = helper.GetRootchainPrivateKey()
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

	// acquire proof for given exit event
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

	// create exit transaction
	txn, exitEvent, err := createExitTxn(senderKey.Address(), proof)
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to create tx input: %w", err))

		return
	}

	// send exit transaction
	receipt, err := rootTxRelayer.SendTransaction(txn, senderKey)
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

// runExitPreRun is used to validate input values
func runExitPreRun(_ *cobra.Command, _ []string) error {
	if err := ep.validateFlags(); err != nil {
		return err
	}

	return nil
}

// createExitTxn encodes parameters for exit function on root chain ExitHelper contract
func createExitTxn(sender ethgo.Address, proof types.Proof) (*ethgo.Transaction, *polybft.ExitEvent, error) {
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
		From:  sender,
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

	buffer.WriteString("\n[EXIT TRANSACTION RELAYER]\n")
	buffer.WriteString(cmdHelper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
