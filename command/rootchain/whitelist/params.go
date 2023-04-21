package whitelist

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

const (
	newValidatorAddressesFlag = "addresses"
)

var (
	errNoNewValidatorsProvided = errors.New("no new validators addresses provided")
)

type whitelistParams struct {
	privateKey             string
	jsonRPC                string
	newValidatorAddresses  []string
	supernetManagerAddress string
}

func (ep *whitelistParams) validateFlags() error {
	if len(ep.newValidatorAddresses) == 0 {
		return errNoNewValidatorsProvided
	}

	return nil
}

type whitelistResult struct {
	newValidatorAddresses []string
}

func (wr whitelistResult) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, len(wr.newValidatorAddresses))

	buffer.WriteString("\n[WHITELIST VALIDATOR]\n")

	for i, addr := range wr.newValidatorAddresses {
		vals[i] = fmt.Sprintf("Validator address|%s", addr)
	}

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
