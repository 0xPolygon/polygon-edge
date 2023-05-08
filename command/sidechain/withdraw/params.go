package withdraw

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

type withdrawParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
}

type withdrawResult struct {
	validatorAddress string
	amount           uint64
}

func (v *withdrawParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
}

func (ur withdrawResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[WITHDRAWAL]\n")

	vals := make([]string, 0, 2)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", ur.validatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Withdrawn|%v", ur.amount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
