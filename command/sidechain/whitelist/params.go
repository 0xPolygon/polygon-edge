package whitelist

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

var (
	newValidatorAddressFlag = "address"
)

type whitelistParams struct {
	accountDir          string
	accountConfig       string
	jsonRPC             string
	newValidatorAddress string
}

func (ep *whitelistParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(ep.accountDir, ep.accountConfig)
}

type enlistResult struct {
	newValidatorAddress string
}

func (er enlistResult) GetOutput() string {
	var buffer bytes.Buffer

	var vals []string

	buffer.WriteString("\n[ENLIST VALIDATOR]\n")

	vals = append(vals, fmt.Sprintf("Validator Address|%s", er.newValidatorAddress))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
