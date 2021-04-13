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

// Help implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Help() string {
	return ""
}

// Synopsis implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Synopsis() string {
	return ""
}

// Run implements the cli.TxPoolAdd interface
func (p *TxPoolAdd) Run(args []string) int {
	flags := p.FlagSet("txpool add")

	// address types
	var fromRaw, toRaw string

	// big int types
	var valueRaw, gasPriceRaw string

	var nonce, gasLimit uint64

	flags.StringVar(&fromRaw, "from", "", "")
	flags.StringVar(&toRaw, "to", "", "")
	flags.StringVar(&valueRaw, "value", "", "")
	flags.StringVar(&gasPriceRaw, "gasPrice", "0x100000", "")
	flags.Uint64Var(&gasLimit, "gasLimit", 1000000, "")
	flags.Uint64Var(&nonce, "nonce", 0, "")

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
		Value:    value.Bytes(),
		GasPrice: gasPrice.Bytes(),
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
