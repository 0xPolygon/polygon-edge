package command

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/txpool/proto"
	txpoolOp "github.com/0xPolygon/minimal/txpool/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/any"
)

// TxPoolAdd is the command to query the snapshot
type TxPoolAdd struct {
	Meta
}

// DefineFlags defines the command flags
func (p *TxPoolAdd) DefineFlags() {
	if p.flagMap == nil {
		// Flag map not initialized
		p.flagMap = make(map[string]FlagDescriptor)
	}

	if len(p.flagMap) > 0 {
		// No need to redefine the flags again
		return
	}

	p.flagMap["from"] = FlagDescriptor{
		description: "The sender address",
		arguments: []string{
			"ADDRESS",
		},
		argumentsOptional: false,
	}

	p.flagMap["to"] = FlagDescriptor{
		description: "The receiver address",
		arguments: []string{
			"ADDRESS",
		},
		argumentsOptional: false,
	}

	p.flagMap["value"] = FlagDescriptor{
		description: "The value of the transaction",
		arguments: []string{
			"VALUE",
		},
		argumentsOptional: false,
	}

	p.flagMap["gasPrice"] = FlagDescriptor{
		description: "The gas price",
		arguments: []string{
			"GASPRICE",
		},
		argumentsOptional: false,
	}

	p.flagMap["gasLimit"] = FlagDescriptor{
		description: "The specified gas limit",
		arguments: []string{
			"LIMIT",
		},
		argumentsOptional: false,
	}

	p.flagMap["nonce"] = FlagDescriptor{
		description: "The nonce of the transaction",
		arguments: []string{
			"NONCE",
		},
		argumentsOptional: false,
	}
}

// GetHelperText returns a simple description of the command
func (p *TxPoolAdd) GetHelperText() string {
	return "Adds a new transaction to the transaction pool"
}

// Help implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Help() string {
	p.DefineFlags()
	usage := "txpool add --from ADDRESS --to ADDRESS --value VALUE\n\t--gasPrice GASPRICE [--gasLimit LIMIT] [--nonce NONCE]"

	return p.GenerateHelp(p.Synopsis(), usage)
}

// Synopsis implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Synopsis() string {
	return p.GetHelperText()
}

// Run implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Run(args []string) int {
	flags := p.FlagSet("txpool add")

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
		p.UI.Error(fmt.Sprintf("failed to decode from address: %v", err))
		return 1
	}
	to := types.Address{}
	if err := to.UnmarshalText([]byte(toRaw)); err != nil {
		p.UI.Error(fmt.Sprintf("failed to decode to address: %v", err))
		return 1
	}
	value, err := types.ParseUint256orHex(&valueRaw)
	if err != nil {
		p.UI.Error(fmt.Sprintf("failed to decode to value: %v", err))
		return 1
	}
	gasPrice, err := types.ParseUint256orHex(&gasPriceRaw)
	if err != nil {
		p.UI.Error(fmt.Sprintf("failed to decode to gasPrice: %v", err))
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
		V:        1, // it is necessary to encode in rlp
	}

	msg := &proto.AddTxnReq{
		Raw: &any.Any{
			Value: txn.MarshalRLP(),
		},
		// from is not encoded in the rlp
		From: from.String(),
	}
	if _, err := clt.AddTxn(context.Background(), msg); err != nil {
		p.UI.Error(fmt.Sprintf("failed to add txn: %v", err))
		return 1
	}
	return 0
}
