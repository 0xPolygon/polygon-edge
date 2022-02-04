package add

import (
	"context"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/output"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
)

// Define local flag values
var (
	from     string
	to       string
	value    string
	gasPrice string
	gasLimit uint64
	nonce    uint64
)

func GetCommand() *cobra.Command {
	txPoolAddCmd := &cobra.Command{
		Use:   "add",
		Short: "Adds a transaction to the transaction pool",
		Run:   runCommand,
	}

	setFlags(txPoolAddCmd)
	setRequiredFlags(txPoolAddCmd)

	return txPoolAddCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&from, "from", "", "the sender address")
	cmd.Flags().StringVar(&to, "to", "", "the receiver address")
	cmd.Flags().StringVar(&value, "value", "", "the value of the transaction")
	cmd.Flags().StringVar(&gasPrice, "gas-price", "0x100000", "the gas price")
	cmd.Flags().Uint64Var(&gasLimit, "gas-limit", 1000000, "the specified gas limit")
	cmd.Flags().Uint64Var(&nonce, "nonce", 0, "the nonce of the transaction")
}

func setRequiredFlags(cmd *cobra.Command) {
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("value")
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := output.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	addParams := &addParams{
		gas:   gasLimit,
		nonce: nonce,
	}

	if err := addParams.init(); err != nil {
		outputter.SetError(err)

		return
	}

	resp, err := addTransaction(
		addParams.constructAddRequest(),
		helper.GetGRPCAddress(cmd),
	)
	if err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(&TxPoolAddResult{
		Hash:     resp.TxHash,
		From:     addParams.from.String(),
		To:       addParams.to.String(),
		Value:    *types.EncodeBigInt(addParams.value),
		GasPrice: *types.EncodeBigInt(addParams.gasPrice),
		GasLimit: addParams.gas,
	})
}

func addTransaction(
	txnRequest *txpoolOp.AddTxnReq,
	grpcAddress string,
) (*txpoolOp.AddTxnResp, error) {
	client, err := helper.GetTxPoolClientConnection(
		grpcAddress,
	)
	if err != nil {
		return nil, err
	}

	resp, err := client.AddTxn(
		context.Background(),
		txnRequest,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add transaction: %w", err)
	}

	return resp, nil
}
