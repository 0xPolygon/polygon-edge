package deploy

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
)

type deployContractResult struct {
	Name    string        `json:"name"`
	Address types.Address `json:"address"`
	Hash    types.Hash    `json:"hash"`
}

func newDeployContractsResult(name string, address types.Address, hash ethgo.Hash) *deployContractResult {
	return &deployContractResult{
		Name:    name,
		Address: address,
		Hash:    types.BytesToHash(hash.Bytes()),
	}
}

func (r deployContractResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[ROOTCHAIN - DEPLOY CONTRACT]\n")

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Name|%s", r.Name))
	vals = append(vals, fmt.Sprintf("Contract (address)|%s", r.Address))
	vals = append(vals, fmt.Sprintf("Transaction (hash)|%s", r.Hash))

	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}

type messageResult struct {
	Message string `json:"message"`
}

func (r messageResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString(r.Message)
	buffer.WriteString("\n")

	return buffer.String()
}
