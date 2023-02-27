package sendtx

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/0xPolygon/polygon-edge/command"
	service "github.com/0xPolygon/polygon-edge/command/aarelayer/service"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
)

const (
	relayerAddrFlag    = "addr"
	chainIDFlag        = "chain-id"
	nonceFlag          = "nonce" // TODO: call actual json rpc of egde node to retrieve nonce
	txFlag             = "tx"
	waitForReceiptFlag = "wait-for-receipt"
	invokerAddrFlag    = "invoker-addr"

	defaultPort = 8198
)

type aarelayerSendTxParams struct {
	addr           string
	accountDir     string
	configPath     string
	chainID        int64
	nonce          uint64
	txs            []string
	waitForReceipt bool
	invokerAddr    string

	payloads []*service.Payload
}

func (rp *aarelayerSendTxParams) validateFlags() error {
	if !helper.ValidateIPPort(rp.addr) {
		return fmt.Errorf("invalid address: %s", rp.addr)
	}

	if len(rp.txs) == 0 {
		return errors.New("at least one transaction should be specified")
	}

	for i, tx := range rp.txs {
		var (
			value    = new(big.Int)
			gasLimit = new(big.Int)
			input    []byte
			err      error
		)

		parts := strings.Split(tx, ":")
		if len(parts) != 3 && len(parts) != 4 {
			return fmt.Errorf("invalid transaction: %d", i)
		}

		if len(strings.TrimPrefix(parts[0], "0x")) != types.AddressLength*2 { // each byte is two hexa chars
			return fmt.Errorf("invalid transaction: %d, wrong address: %s", i, parts[0])
		}

		// value (can be specified as hex or as a decimal)
		if _, ok := value.SetString(parts[1], 0); !ok {
			return fmt.Errorf("invalid transaction: %d, wrong value: %s", i, parts[1])
		}

		// gaslimit (can be specified as hex or as a decimal)
		if _, ok := gasLimit.SetString(parts[2], 0); !ok {
			return fmt.Errorf("invalid transaction: %d, wrong gas limit: %s", i, parts[2])
		}

		// input
		if len(parts) == 4 {
			input, err = hex.DecodeString(strings.TrimPrefix(parts[3], "0x"))
			if err != nil {
				return fmt.Errorf("invalid transaction: %d, wrong input: %s", i, parts[3])
			}
		} else {
			input = framework.MethodSig("increment")
		}

		to := types.StringToAddress(parts[0])

		rp.payloads = append(rp.payloads, &service.Payload{
			To:       &to,
			Value:    value,
			GasLimit: gasLimit,
			Input:    input,
		})
	}

	return sidechainHelper.ValidateSecretFlags(rp.accountDir, rp.configPath)
}

func (rp *aarelayerSendTxParams) createAATransaction(key ethgo.Key) (*service.AATransaction, error) {
	aaTx := &service.AATransaction{
		Transaction: service.Transaction{
			From:    types.Address(key.Address()),
			Nonce:   rp.nonce,
			Payload: make([]service.Payload, len(rp.payloads)),
		},
	}

	for i, payload := range rp.payloads {
		aaTx.Transaction.Payload[i] = service.Payload{
			To:       payload.To,
			Value:    payload.Value,
			GasLimit: payload.GasLimit,
			Input:    payload.Input,
		}
	}

	invokerAddress := types.StringToAddress(rp.invokerAddr)

	if err := aaTx.MakeSignature(invokerAddress, rp.chainID, key); err != nil {
		return nil, err
	}

	return aaTx, nil
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.addr,
		relayerAddrFlag,
		fmt.Sprintf("%s:%d", helper.AllInterfacesBinding, defaultPort),
		"rest server address [ip:port]",
	)

	cmd.Flags().StringVar(
		&params.accountDir,
		polybftsecrets.DataPathFlag,
		"",
		polybftsecrets.DataPathFlagDesc,
	)

	cmd.Flags().StringVar(
		&params.configPath,
		polybftsecrets.ConfigFlag,
		"",
		polybftsecrets.ConfigFlagDesc,
	)

	cmd.Flags().Int64Var(
		&params.chainID,
		chainIDFlag,
		command.DefaultChainID,
		"the ID of the chain",
	)

	cmd.Flags().Uint64Var(
		&params.nonce,
		nonceFlag,
		0,
		"Nonce for the first transaction",
	)

	cmd.Flags().StringArrayVar(
		&params.txs,
		txFlag,
		[]string{},
		"transaction <to:value:gaslimit[:input]>",
	)

	cmd.Flags().StringVar(
		&params.invokerAddr,
		invokerAddrFlag,
		service.DefaultAAInvokerAddress.String(),
		"address of invoker smart contract",
	)

	cmd.Flags().BoolVar(
		&params.waitForReceipt,
		waitForReceiptFlag,
		true,
		"should command wait for receipt or not (default is true)",
	)

	cmd.MarkFlagsMutuallyExclusive(polybftsecrets.ConfigFlag, polybftsecrets.DataPathFlag)
}
