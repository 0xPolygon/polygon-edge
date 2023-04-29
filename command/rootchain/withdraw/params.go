package withdraw

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

var (
	addressToFlag = "to"
)

type withdrawParams struct {
	accountDir       string
	accountConfig    string
	jsonRPC          string
	stakeManagerAddr string
	addressTo        string
	amount           string

	amountValue *big.Int
}

func (v *withdrawParams) validateFlags() (err error) {
	if v.amountValue, err = helper.ParseAmount(v.amount); err != nil {
		return err
	}

	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
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
