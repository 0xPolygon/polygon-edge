package withdraw

import (
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
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
	*common.BridgeParams
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
		Use:     "exit",
		Short:   "Performs exit transaction from the child chain to the root chain",
		PreRunE: runPreRunExit,
		Run:     runExitCommand,
	}

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
		"the JSON RPC root chain endpoint (e.g. http://127.0.0.1:8545)",
	)

	exitCmd.Flags().StringVar(
		&ep.childJSONRPCAddr,
		childJSONRPCFlag,
		"http://127.0.0.1:9545",
		"the JSON RPC child chain endpoint (e.g. http://127.0.0.1:9545)",
	)

	exitCmd.MarkFlagRequired(exitHelperFlag)
	exitCmd.MarkFlagRequired(rootJSONRPCFlag)
	exitCmd.MarkFlagRequired(childJSONRPCFlag)

	return exitCmd
}

func runPreRunExit(cmd *cobra.Command, _ []string) error {
	sharedParams, err := common.GetBridgeParams(cmd)
	if err != nil {
		return err
	}

	ep.BridgeParams = sharedParams

	if err = ep.ValidateFlags(); err != nil {
		return err
	}

	return nil
}

func runExitCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	ecdsaRaw, err := hex.DecodeString(ep.TxnSenderKey)
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

	err = childClient.Call(generateExitProofFn, proof,
		fmt.Sprintf("0x%x", ep.exitID),
		fmt.Sprintf("0x%x", ep.epochNumber),
		fmt.Sprintf("0x%x", ep.checkpointBlock))
	if err != nil {
		outputter.SetError(fmt.Errorf("failed to get exit proof (exit id=%d, epoch number=%d, checkpoint block=%d): %w",
			ep.exitID, ep.epochNumber, ep.checkpointBlock, err))

		return
	}

	outputter.Write([]byte(fmt.Sprintf("Proof: %+v\n", proof)))

	// exit transaction
	txn, err := createExitTxn(proof, outputter)
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
}

// createExitTxn encodes parameters for exit function on root chain ExitHelper contract
func createExitTxn(proof types.Proof, output command.OutputFormatter) (*ethgo.Transaction, error) {
	output.Write([]byte(fmt.Sprintf("Proof.ID type %v\n", reflect.TypeOf(proof.Metadata["ID"]))))
	output.Write([]byte(fmt.Sprintf("Proof.Sender type %v\n", reflect.TypeOf(proof.Metadata["Sender"]))))
	output.Write([]byte(fmt.Sprintf("Proof.Receiver type %v\n", reflect.TypeOf(proof.Metadata["Receiver"]))))
	output.Write([]byte(fmt.Sprintf("Proof.Data type %v\n", reflect.TypeOf(proof.Metadata["Data"]))))

	exitEventEncoded, err := polybft.ExitEventABIType.Encode(&polybft.ExitEvent{
		ID:       uint64(proof.Metadata["ID"].(float64)),
		Sender:   ethgo.Address(types.BytesToAddress(proof.Metadata["Sender"].([]byte))),
		Receiver: ethgo.Address(types.BytesToAddress(proof.Metadata["Receiver"].([]byte))),
		Data:     proof.Metadata["Data"].([]byte),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode exit event: %w", err)
	}

	input, err := contractsapi.ExitHelper.Abi.Methods["exit"].Encode([]interface{}{
		ep.checkpointBlock,
		proof.Metadata["LeafIndex"].(float64),
		exitEventEncoded,
		proof.Data,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode provided parameters: %w", err)
	}

	addr := ethgo.Address(types.StringToAddress(ep.exitHelperAddrRaw))

	return &ethgo.Transaction{
		To:    &addr,
		Input: input,
	}, nil
}
