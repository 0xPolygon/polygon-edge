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
	if _, err := helper.ParseJSONRPCAddress(w.jsonRPC); err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	return sidechainHelper.ValidateSecretFlags(w.accountDir, w.accountConfig)
}

type withdrawResult struct {
	ValidatorAddress string     `json:"validatorAddress"`
	Amount           *big.Int   `json:"amount"`
	ExitEventIDs     []*big.Int `json:"exitEventIDs"`
	BlockNumber      uint64     `json:"blockNumber"`
}

func (r *withdrawResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[WITHDRAWAL]\n")

	vals := make([]string, 0, 4)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", r.ValidatorAddress))
	vals = append(vals, fmt.Sprintf("Amount Withdrawn|%d", r.Amount))
	vals = append(vals, fmt.Sprintf("Exit Event IDs|%d", r.ExitEventIDs))
	vals = append(vals, fmt.Sprintf("Inclusion Block Number|%d", r.BlockNumber))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
