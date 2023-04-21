package staking

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

type stakeParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
	amount        uint64
	chainID       uint64
}

func (v *stakeParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
}

type stakeResult struct {
	validatorAddress string
	amount           uint64
}

func (sr stakeResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[VALIDATOR STAKE]\n")

	vals := make([]string, 0, 2)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", sr.validatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Staked|%v", sr.amount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
