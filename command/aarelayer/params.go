package aarelayer

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

const (
	addrFlag = "addr"

	defaultPort = 8081
)

type aarelayerParams struct {
	addr string
}

func (rp *aarelayerParams) validateFlags() error {
	if !helper.ValidateIPPort(rp.addr) {
		return fmt.Errorf("invalid address: %s", rp.addr)
	}

	return nil
}
