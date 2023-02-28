package staking

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

var (
	delegateAddressFlag = "delegate"
)

type stakeParams struct {
	accountDir      string
	configPath      string
	jsonRPC         string
	amount          uint64
	self            bool
	delegateAddress string
}

func (v *stakeParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.configPath)
}

type stakeResult struct {
	validatorAddress string
	isSelfStake      bool
	amount           uint64
	delegatedTo      string
}

func (sr stakeResult) GetOutput() string {
	var buffer bytes.Buffer

	var vals []string

	if sr.isSelfStake {
		buffer.WriteString("\n[SELF STAKE]\n")

		vals = make([]string, 0, 2)
		vals = append(vals, fmt.Sprintf("Validator Address|%s", sr.validatorAddress))
		vals = append(vals, fmt.Sprintf("Amount Staked|%v", sr.amount))
	} else {
		buffer.WriteString("\n[DELEGATED AMOUNT]\n")

		vals = make([]string, 0, 3)
		vals = append(vals, fmt.Sprintf("Validator Address|%s", sr.validatorAddress))
		vals = append(vals, fmt.Sprintf("Amount Delegated|%v", sr.amount))
		vals = append(vals, fmt.Sprintf("Delegated To|%s", sr.delegatedTo))
	}

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
