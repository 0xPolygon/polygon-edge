package unstaking

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

var (
	undelegateAddressFlag = "undelegate"
)

type unstakeParams struct {
	accountDir        string
	accountConfig     string
	jsonRPC           string
	amount            uint64
	self              bool
	undelegateAddress string
}

func (v *unstakeParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
}

type unstakeResult struct {
	validatorAddress string
	isSelfUnstake    bool
	amount           uint64
	undelegatedFrom  string
}

func (ur unstakeResult) GetOutput() string {
	var buffer bytes.Buffer

	var vals []string

	if ur.isSelfUnstake {
		buffer.WriteString("\n[SELF UNSTAKE]\n")

		vals = make([]string, 0, 2)
		vals = append(vals, fmt.Sprintf("Validator Address|%s", ur.validatorAddress))
		vals = append(vals, fmt.Sprintf("Amount Unstaked|%v", ur.amount))
	} else {
		buffer.WriteString("\n[UNDELEGATED AMOUNT]\n")

		vals = make([]string, 0, 3)
		vals = append(vals, fmt.Sprintf("Validator Address|%s", ur.validatorAddress))
		vals = append(vals, fmt.Sprintf("Amount Undelegated|%v", ur.amount))
		vals = append(vals, fmt.Sprintf("Undelegated From|%s", ur.undelegatedFrom))
	}

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
