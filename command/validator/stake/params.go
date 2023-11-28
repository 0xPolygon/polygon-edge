package staking

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/helper"
	validatorHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
)

var supernetIDFlag = "supernet-id"

type stakeParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
	amount        string

	amountValue *big.Int
}

func (sp *stakeParams) validateFlags() (err error) {
	if sp.amountValue, err = helper.ParseAmount(sp.amount); err != nil {
		return err
	}

	// validate jsonrpc address
	if _, err := helper.ParseJSONRPCAddress(sp.jsonRPC); err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	return validatorHelper.ValidateSecretFlags(sp.accountDir, sp.accountConfig)
}

type stakeResult struct {
	ValidatorAddress string   `json:"validatorAddress"`
	Amount           *big.Int `json:"amount"`
}

func (sr stakeResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[VALIDATOR STAKE]\n")

	vals := make([]string, 0, 2)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", sr.ValidatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Staked|%d", sr.Amount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
