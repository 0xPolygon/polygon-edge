package unstaking

import (
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/sidechain"
)

var (
	undelegateAddressFlag = "undelegate"
)

type unstakeParams struct {
	accountDir        string
	jsonRPC           string
	amount            uint64
	self              bool
	undelegateAddress string
}

func (v *unstakeParams) validateFlags() error {
	return sidechainHelper.CheckIfDirectoryExist(v.accountDir)
}
