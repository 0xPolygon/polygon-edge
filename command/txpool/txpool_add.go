package txpool

import (
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	txpoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	any "google.golang.org/protobuf/types/known/anypb"
)

// TxPoolAdd is the command to query the snapshot
type TxPoolAdd struct {
	helper.Meta
}

// DefineFlags defines the command flags
func (p *TxPoolAdd) DefineFlags() {
	if p.FlagMap == nil {
		// Flag map not initialized
		p.FlagMap = make(map[string]helper.FlagDescriptor)
	}

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
	p.Meta.DefineFlags()
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Run(args []string) int {
	flags := p.FlagSet(p.GetBaseCommand())

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
		p.UI.Error(fmt.Sprintf("Failed to decode from address: %v", err))
		return 1
	}
	to := types.Address{}
	if err := to.UnmarshalText([]byte(toRaw)); err != nil {
		p.UI.Error(fmt.Sprintf("Failed to decode to address: %v", err))
		return 1
	}
	value, err := types.ParseUint256orHex(&valueRaw)
	if err != nil {
		p.UI.Error(fmt.Sprintf("Failed to decode to value: %v", err))
		return 1
	}
	gasPrice, err := types.ParseUint256orHex(&gasPriceRaw)
	if err != nil {
		p.UI.Error(fmt.Sprintf("Failed to decode to gasPrice: %v", err))
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	clt := txpoolOp.NewTxnPoolOperatorClient(conn)

	txn := &types.Transaction{
		To:       &to,
		Gas:      gasLimit,
		Value:    value,
		GasPrice: gasPrice,
		Nonce:    nonce,
		V:        []byte{1}, // it is necessary to encode in rlp
	}

	msg := &proto.AddTxnReq{
		Raw: &any.Any{
			Value: txn.MarshalRLP(),
		},
		// from is not encoded in the rlp
		From: from.String(),
	}

	if _, err := clt.AddTxn(context.Background(), msg); err != nil {
		p.UI.Error(fmt.Sprintf("Failed to add transaction: %v", err))
		return 1
	}

	output := "\n[ADD TRANSACTION]\n"
	output += "Successfully added transaction:\n"

	output += helper.FormatKV([]string{
		fmt.Sprintf("FROM|%s", fromRaw),
		fmt.Sprintf("TO|%s", toRaw),
		fmt.Sprintf("VALUE|%s", valueRaw),
		fmt.Sprintf("GAS PRICE|%s", gasPriceRaw),
		fmt.Sprintf("GAS LIMIT|%d", gasLimit),
	})

	output += "\n"

	p.UI.Info(output)

	return 0
}
