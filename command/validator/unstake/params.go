package unstaking

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/helper"
	validatorHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
)

var (
	undelegateAddressFlag = "undelegate"
)

type unstakeParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
	amount        string
	txTimeout     uint64
	txPollFreq    uint64

	amountValue *big.Int
}

func (v *unstakeParams) validateFlags() (err error) {
	if v.amountValue, err = helper.ParseAmount(v.amount); err != nil {
		return err
	}

	if _, err = helper.ParseJSONRPCAddress(v.jsonRPC); err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	return validatorHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
}

type unstakeResult struct {
	ValidatorAddress string `json:"validatorAddress"`
	Amount           uint64 `json:"amount"`
}

func (ur unstakeResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[UNSTAKE]\n")

	vals := make([]string, 0, 2)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", ur.ValidatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Unstaked|%v", ur.Amount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
