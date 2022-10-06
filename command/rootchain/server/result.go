package server

import (
	"bytes"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

type containerStopResult struct {
	Status int64  `json:"status"`
	Err    string `json:"err"`
}

func newContainerStopResult(status container.ContainerWaitOKBody) *containerStopResult {
	var errMsg string
	if status.Error != nil {
		errMsg = status.Error.Message
	}

	return &containerStopResult{
		Status: status.StatusCode,
		Err:    errMsg,
	}
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

func newInitialDeployResult(name string, address types.Address, hash ethgo.Hash) *initialDeployResult {
	return &initialDeployResult{
		Name:    name,
		Address: address,
		Hash:    types.BytesToHash(hash.Bytes()),
	}
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
