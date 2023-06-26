package withdraw

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

type withdrawParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
}

func (w *withdrawParams) validateFlags() error {
	return sidechainHelper.ValidateSecretFlags(w.accountDir, w.accountConfig)
}

type withdrawResult struct {
	validatorAddress string
	amount           *big.Int
	exitEventID      *big.Int
	blockNumber      uint64
}

func (r *withdrawResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[WITHDRAWAL]\n")

	vals := make([]string, 0, 4)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", r.validatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Withdrawn|%d", r.amount))
	vals = append(vals, fmt.Sprintf("Exit Event ID|%d", r.exitEventID))
	vals = append(vals, fmt.Sprintf("Inclusion Block Number|%d", r.blockNumber))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
