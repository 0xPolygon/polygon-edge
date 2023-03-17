package common

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
)

const (
	ReceiversFlag = "receivers"
	AmountsFlag   = "amounts"
)

var (
	errInconsistentAccounts = errors.New("receivers and amounts must be equal length")
)

type ERC20BridgeParams struct {
	AccountDir    string
	AccountConfig string
	Receivers     []string
	Amounts       []string
}

func (bp *ERC20BridgeParams) ValidateFlags(isTestMode bool) error {
	// in case of test mode test rootchain account is being used as the rootchain transactions sender
	if err := helper.ValidateSecretFlags(isTestMode, bp.AccountDir, bp.AccountConfig); err != nil {
		return err
	}

	if len(bp.Receivers) != len(bp.Amounts) {
		return errInconsistentAccounts
	}

	return nil
}
