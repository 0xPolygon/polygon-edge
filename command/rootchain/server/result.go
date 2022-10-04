package server

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"

	"github.com/0xPolygon/polygon-edge/types"
)

type containerStopResult struct {
	Status int64  `json:"status"`
	Err    string `json:"err"`
}

func (r containerStopResult) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 2)
	vals = append(vals, fmt.Sprintf("Status|%d", r.Status))
	vals = append(vals, fmt.Sprintf("Error|%s", r.Err))

	buffer.WriteString("\n[ROOTCHAIN SERVER - STOP]\n")
	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}

type initialDeployResult struct {
	Name    string        `json:"name"`
	Address types.Address `json:"address"`
	Hash    types.Hash    `json:"hash"`
}

func (r initialDeployResult) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Name|%s", r.Name))
	vals = append(vals, fmt.Sprintf("Contract (address)|%s", r.Address))
	vals = append(vals, fmt.Sprintf("Transaction (hash)|%s", r.Hash))

	buffer.WriteString("\n[ROOTCHAIN SERVER - DEPLOY CONTRACT]\n")
	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
