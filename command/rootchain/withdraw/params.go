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

	if _, err = helper.ParseJSONRPCAddress(v.jsonRPC); err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	return sidechainHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
}

type withdrawResult struct {
	ValidatorAddress string `json:"validatorAddress"`
	Amount           uint64 `json:"amount"`
	WithdrawnTo      string `json:"withdrawnTo"`
}

func (wr withdrawResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[WITHDRAWN AMOUNT]\n")

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", wr.ValidatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Withdrawn|%v", wr.Amount))
	vals = append(vals, fmt.Sprintf("Withdrawn To|%s", wr.WithdrawnTo))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
