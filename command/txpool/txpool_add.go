package txpool

import (
	"bytes"
	"context"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	txpoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	any "google.golang.org/protobuf/types/known/anypb"
)

// TxPoolAdd is the command to query the snapshot
type TxPoolAdd struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (p *TxPoolAdd) DefineFlags() {
	p.Base.DefineFlags(p.Formatter, p.GRPC)

	p.FlagMap["from"] = helper.FlagDescriptor{
		Description: "The sender address",
		Arguments: []string{
			"ADDRESS",
		},
		ArgumentsOptional: false,
	}

	p.FlagMap["to"] = helper.FlagDescriptor{
		Description: "The receiver address",
		Arguments: []string{
			"ADDRESS",
		},
		ArgumentsOptional: false,
	}

	p.FlagMap["value"] = helper.FlagDescriptor{
		Description: "The value of the transaction",
		Arguments: []string{
			"VALUE",
		},
		ArgumentsOptional: false,
	}

	p.FlagMap["gasPrice"] = helper.FlagDescriptor{
		Description: "The gas price",
		Arguments: []string{
			"GASPRICE",
		},
		ArgumentsOptional: false,
	}

	p.FlagMap["gasLimit"] = helper.FlagDescriptor{
		Description: "The specified gas limit",
		Arguments: []string{
			"LIMIT",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	p.FlagMap["nonce"] = helper.FlagDescriptor{
		Description: "The nonce of the transaction",
		Arguments: []string{
			"NONCE",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}
}

// GetHelperText returns a simple description of the command
func (p *TxPoolAdd) GetHelperText() string {
	return "Adds a new transaction to the transaction pool"
}

func (p *TxPoolAdd) GetBaseCommand() string {
	return "txpool add"
}

// Help implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Help() string {
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Run(args []string) int {
	flags := p.Base.NewFlagSet(p.GetBaseCommand(), p.Formatter, p.GRPC)

	// Address types
	var fromRaw, toRaw string

	// BigInt types
	var valueRaw, gasPriceRaw string

	var nonce, gasLimit uint64

	// Define the flags
	flags.StringVar(&fromRaw, "from", "", "")
	flags.StringVar(&toRaw, "to", "", "")
	flags.StringVar(&valueRaw, "value", "", "")
	flags.StringVar(&gasPriceRaw, "gasPrice", "0x100000", "")
	flags.Uint64Var(&gasLimit, "gasLimit", 1000000, "")
	flags.Uint64Var(&nonce, "nonce", 0, "")

	// Save the flags for the help method

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	// try to decode to the custom types (TODO: Use custom flag helpers to decode this)
	from := types.Address{}
	if err := from.UnmarshalText([]byte(fromRaw)); err != nil {
		p.Formatter.OutputError(fmt.Errorf("Failed to decode from address: %v", err))
		return 1
	}
	to := types.Address{}
	if err := to.UnmarshalText([]byte(toRaw)); err != nil {
		p.Formatter.OutputError(fmt.Errorf("Failed to decode to address: %v", err))
		return 1
	}
	value, err := types.ParseUint256orHex(&valueRaw)
	if err != nil {
		p.Formatter.OutputError(fmt.Errorf("Failed to decode to value: %v", err))
		return 1
	}
	gasPrice, err := types.ParseUint256orHex(&gasPriceRaw)
	if err != nil {
		p.Formatter.OutputError(fmt.Errorf("Failed to decode to gasPrice: %v", err))
		return 1
	}

	conn, err := p.GRPC.Conn()
	if err != nil {
		p.Formatter.OutputError(err)
		return 1
	}

	clt := txpoolOp.NewTxnPoolOperatorClient(conn)

	txn := &types.Transaction{
		To:       &to,
		Gas:      gasLimit,
		Value:    value,
		GasPrice: gasPrice,
		Nonce:    nonce,
		V:        big.NewInt(1), // it is necessary to encode in rlp
	}

	msg := &proto.AddTxnReq{
		Raw: &any.Any{
			Value: txn.MarshalRLP(),
		},
		// from is not encoded in the rlp
		From: from.String(),
	}

	if _, err := clt.AddTxn(context.Background(), msg); err != nil {
		p.Formatter.OutputError(fmt.Errorf("Failed to add transaction: %v", err))
		return 1
	}

	res := &TxPoolAddResult{
		From:     fromRaw,
		To:       toRaw,
		Value:    *types.EncodeBigInt(value),
		GasPrice: *types.EncodeBigInt(gasPrice),
		GasLimit: gasLimit,
	}
	p.Formatter.OutputResult(res)

	return 0
}

type TxPoolAddResult struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	GasPrice string `json:"gas_price"`
	GasLimit uint64 `json:"gas_limit"`
}

func (r *TxPoolAddResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[ADD TRANSACTION]\n")
	buffer.WriteString("Successfully added transaction:\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("FROM|%s", r.From),
		fmt.Sprintf("TO|%s", r.To),
		fmt.Sprintf("VALUE|%s", r.Value),
		fmt.Sprintf("GAS PRICE|%s", r.GasPrice),
		fmt.Sprintf("GAS LIMIT|%d", r.GasLimit),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
