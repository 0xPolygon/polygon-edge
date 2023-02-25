package withdraw

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

var (
	addressToFlag = "to"
)

type withdrawParams struct {
	accountDir string
	configPath string
	jsonRPC    string
	addressTo  string
}

func (v *withdrawParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.configPath)
}

type withdrawResult struct {
	validatorAddress string
	amount           uint64
	withdrawnTo      string
}

func (wr withdrawResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[WITHDRAWN AMOUNT]\n")

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", wr.validatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Withdrawn|%v", wr.amount))
	vals = append(vals, fmt.Sprintf("Withdrawn To|%s", wr.withdrawnTo))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
