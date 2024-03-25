package withdraw

import (
	"bytes"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/command/helper"
	validatorHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
)

var (
	addressToFlag = "to"
)

type withdrawParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
	txTimeout     time.Duration
}

func (v *withdrawParams) validateFlags() (err error) {
	if _, err = helper.ParseJSONRPCAddress(v.jsonRPC); err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	return validatorHelper.ValidateSecretFlags(v.accountDir, v.accountConfig)
}

type withdrawResult struct {
	ValidatorAddress string `json:"validatorAddress"`
	Amount           uint64 `json:"amount"`
}

func (wr withdrawResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[WITHDRAWN AMOUNT]\n")

	vals := make([]string, 0, 2)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", wr.ValidatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Withdrawn|%v", wr.Amount))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
