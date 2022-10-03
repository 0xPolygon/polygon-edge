package server

import (
	"bytes"

	"github.com/0xPolygon/polygon-edge/types"
)

type initialDeployResult struct {
	name    string
	address types.Address
	hash    types.Hash
}

type gatherLogsResult struct {
	stdOut bytes.Buffer
	stdErr bytes.Buffer
}

type runResult struct {
	imgPullOut bytes.Buffer
	stopErr    error
	stopStatus int64
}
