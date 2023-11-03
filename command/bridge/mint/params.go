package mint

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/helper"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

type mintParams struct {
	addresses          []string
	amounts            []string
	tokenAddr          string
	deployerPrivateKey string
	jsonRPCAddress     string

	amountValues []*big.Int
}

func (m *mintParams) validateFlags() error {
	if len(m.addresses) == 0 {
		return rootHelper.ErrNoAddressesProvided
	}

	if len(m.amounts) != len(m.addresses) {
		return rootHelper.ErrInconsistentLength
	}

	for _, addr := range m.addresses {
		if err := types.IsValidAddress(addr); err != nil {
			return err
		}
	}

	m.amountValues = make([]*big.Int, len(m.amounts))
	for i, amountRaw := range m.amounts {
		amountValue, err := helper.ParseAmount(amountRaw)
		if err != nil {
			return err
		}

		m.amountValues[i] = amountValue
	}

	if m.tokenAddr == "" {
		return rootHelper.ErrMandatoryERC20Token
	}

	if err := types.IsValidAddress(m.tokenAddr); err != nil {
		return fmt.Errorf("invalid erc20 token address is provided: %w", err)
	}

	return nil
}

type mintResult struct {
	Address types.Address `json:"address"`
	Amount  *big.Int      `json:"amount"`
	TxHash  types.Hash    `json:"tx_hash"`
}

func (m *mintResult) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Address|%s", m.Address))
	vals = append(vals, fmt.Sprintf("Amount|%s", m.Amount))
	vals = append(vals, fmt.Sprintf("Transaction (hash)|%s", m.TxHash))

	buffer.WriteString("\n[MINT-ERC20]\n")
	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
