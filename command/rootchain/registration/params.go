package registration

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

const (
	chainIDFlag = "chain-id"
)

type registerParams struct {
	accountDir             string
	accountConfig          string
	supernetManagerAddress string
	jsonRPC                string
}

func (rp *registerParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(rp.accountDir, rp.accountConfig)
}

type registerResult struct {
	validatorAddress string
}

func (rr registerResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[REGISTRATION]\n")

	vals := make([]string, 0, 1)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", rr.validatorAddress))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
