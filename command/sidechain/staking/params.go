package staking

import (
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

var (
	delegateAddressFlag = "delegate"
)

type stakeParams struct {
	accountDir      string
	jsonRPC         string
	amount          uint64
	self            bool
	delegateAddress string
}

func (v *stakeParams) validateFlags() error {
	return sidechainHelper.CheckIfDirectoryExist(v.accountDir)
}
