package whitelist

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

const (
	newValidatorAddressesFlag = "addresses"
)

var (
	errNoNewValidatorsProvided = errors.New("no new validators addresses provided")
)

type whitelistParams struct {
	accountDir             string
	accountConfig          string
	privateKey             string
	jsonRPC                string
	newValidatorAddresses  []string
	supernetManagerAddress string
}

func (ep *whitelistParams) validateFlags() error {
	if len(ep.newValidatorAddresses) == 0 {
		return errNoNewValidatorsProvided
	}

	if ep.privateKey == "" {
		return sidechainHelper.ValidateSecretFlags(ep.accountDir, ep.accountConfig)
	}

	// validate jsonrpc address
	_, err := helper.ParseJSONRPCAddress(ep.jsonRPC)

	return err
}

type whitelistResult struct {
	providedAddresses     []string
	newValidatorAddresses []string
}

func (wr whitelistResult) GetOutput() string {
	var buffer bytes.Buffer

	var vals []string
	for _, addr := range wr.providedAddresses {
		vals = append(vals, fmt.Sprintf("Validator address|%s", addr))
	}

	buffer.WriteString("\n[WHITELIST PROVIDED VALIDATORS]\n")
	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	buffer.WriteString("\n[WHITELIST VALIDATORS]\n")

	vals = []string{}
	for _, addr := range wr.newValidatorAddresses {
		vals = append(vals, fmt.Sprintf("Validator address|%s", addr))
	}

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
