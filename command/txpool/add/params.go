package add

import (
	"fmt"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/golang/protobuf/ptypes/any"
	"math/big"
)

const (
	fromFlag     = "from"
	toFlag       = "to"
	valueFlag    = "value"
	gasPriceFlag = "gas-price"
	gasLimitFlag = "gas-limit"
	nonceFlag    = "nonce"
)

var (
	params = &addParams{}
)

type addParams struct {
	fromRaw     string
	toRaw       string
	valueRaw    string
	gasPriceRaw string

	from types.Address
	to   types.Address

	gas   uint64
	nonce uint64

	value    *big.Int
	gasPrice *big.Int
}

func (ap *addParams) getRequiredFlags() []string {
	return []string{
		fromFlag,
		toFlag,
		valueFlag,
	}
}

func (ap *addParams) init() error {
	if err := ap.initAddressValues(); err != nil {
		return err
	}

	if err := ap.initUintOrHexValues(); err != nil {
		return err
	}

	return nil
}

func (ap *addParams) initAddressValues() (err error) {
	ap.from, err = getAddressFromString(ap.fromRaw)
	if err != nil {
		return
	}

	ap.to, err = getAddressFromString(ap.toRaw)
	if err != nil {
		return
	}

	return
}

func (ap *addParams) initUintOrHexValues() (err error) {
	ap.value, err = types.ParseUint256orHex(&ap.valueRaw)
	if err != nil {
		return
	}

	ap.gasPrice, err = types.ParseUint256orHex(&ap.gasPriceRaw)
	if err != nil {
		return
	}

	return
}

func (ap *addParams) constructAddRequest() *txpoolOp.AddTxnReq {
	txn := &types.Transaction{
		To:       &ap.to,
		Gas:      ap.gas,
		Value:    ap.value,
		GasPrice: ap.gasPrice,
		Nonce:    ap.nonce,
		V:        big.NewInt(1), // it is necessary to encode in rlp
	}

	return &txpoolOp.AddTxnReq{
		Raw: &any.Any{
			Value: txn.MarshalRLP(),
		},
		// from is not encoded in the rlp
		From: ap.from.String(),
	}
}

func getAddressFromString(input string) (types.Address, error) {
	a := types.Address{}
	if err := a.UnmarshalText([]byte(input)); err != nil {
		return types.ZeroAddress,
			fmt.Errorf("failed to decode from address: %w", err)
	}

	return a, nil
}
