package unstaking

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

var (
	undelegateAddressFlag = "undelegate"
)

type unstakeParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
	amount        string

	amountValue *big.Int
}

func (v *unstakeParams) validateFlags() (err error) {
	if v.amountValue, err = helper.ParseAmount(v.amount); err != nil {
		return err
	}

	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
}

type unstakeResult struct {
	validatorAddress string
	amount           uint64
}

func (ur unstakeResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[UNSTAKE]\n")

	vals := make([]string, 0, 2)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", ur.validatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Unstaked|%v", ur.amount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
