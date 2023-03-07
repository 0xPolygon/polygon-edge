package common

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/command/sidechain"
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

func (bp *ERC20BridgeParams) ValidateFlags(testMode bool) error {
	// in case of test mode test rootchain account is being used as the rootchain transactions sender
	if !testMode {
		if err := sidechain.ValidateSecretFlags(bp.AccountDir, bp.AccountConfig); err != nil {
			return err
		}
	} else {
		if bp.AccountDir != "" || bp.AccountConfig != "" {
			return helper.ErrTestModeSecrets
		}
	}

	if len(bp.Receivers) != len(bp.Amounts) {
		return errInconsistentAccounts
	}

	return nil
}
