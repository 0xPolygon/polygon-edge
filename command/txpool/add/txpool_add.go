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
	cmd.Flags().StringVar(
		&params.fromRaw,
		fromFlag,
		"",
		"the sender address",
	)

	cmd.Flags().StringVar(
		&params.toRaw,
		toFlag,
		"",
		"the receiver address",
	)

	cmd.Flags().StringVar(
		&params.valueRaw,
		valueFlag,
		"",
		"the value of the transaction",
	)

	cmd.Flags().StringVar(
		&params.gasPriceRaw,
		gasPriceFlag,
		"0x100000",
		"the gas price",
	)

	cmd.Flags().Uint64Var(
		&params.gas,
		gasLimitFlag,
		1000000,
		"the specified gas limit",
	)

	cmd.Flags().Uint64Var(
		&params.nonce,
		nonceFlag,
		0,
		"the nonce of the transaction",
	)
}

func setRequiredFlags(cmd *cobra.Command) {
	for _, requiredFlag := range params.getRequiredFlags() {
		_ = cmd.MarkFlagRequired(requiredFlag)
	}
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := output.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.init(); err != nil {
		outputter.SetError(err)

		return
	}

	resp, err := addTransaction(
		params.constructAddRequest(),
		helper.GetGRPCAddress(cmd),
	)
	if err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(&TxPoolAddResult{
		Hash:     resp.TxHash,
		From:     params.from.String(),
		To:       params.to.String(),
		Value:    *types.EncodeBigInt(params.value),
		GasPrice: *types.EncodeBigInt(params.gasPrice),
		GasLimit: params.gas,
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
