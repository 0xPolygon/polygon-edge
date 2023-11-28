package server

import (
	"bytes"
	"fmt"

	"github.com/docker/docker/api/types/container"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type containerStopResult struct {
	Status int64  `json:"status"`
	Err    string `json:"err"`
}

func newContainerStopResult(status container.WaitResponse) *containerStopResult {
	errMsg := ""
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
