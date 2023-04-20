package whitelist

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

var (
	newValidatorAddressesFlag  = "new-validators-addr"
	errNoNewValidatorsProvided = errors.New("no new validators addresses provided")
)

type whitelistParams struct {
	accountDir             string
	accountConfig          string
	jsonRPC                string
	newValidatorAddresses  []string
	supernetManagerAddress string
}

func (ep *whitelistParams) validateFlags() error {
	if len(ep.newValidatorAddresses) == 0 {
		return errNoNewValidatorsProvided
	}

	return sidechainHelper.ValidateSecretFlags(ep.accountDir, ep.accountConfig)
}

type enlistResult struct {
	newValidatorAddresses []string
}

func (er enlistResult) GetOutput() string {
	var buffer bytes.Buffer

	var vals []string

	buffer.WriteString("\n[ENLIST VALIDATOR]\n")

	for _, addr := range er.newValidatorAddresses {
		vals = append(vals, fmt.Sprintf("Validator address|%s", addr))
	}

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
