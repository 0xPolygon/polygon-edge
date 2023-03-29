package common

import (
	"errors"
)

type TokenType int

const (
	ERC20 TokenType = iota
	ERC721
	ERC1155
)

const (
	SenderKeyFlag = "sender-key"
	ReceiversFlag = "receivers"
	AmountsFlag   = "amounts"
	TokenIDsFlag  = "token-ids"
)

var (
	errInconsistentAccounts = errors.New("receivers and amounts must be equal length")
)

type ERC20BridgeParams struct {
	SenderKey string
	Receivers []string
	Amounts   []string
}

func (bp *ERC20BridgeParams) ValidateFlags() error {
	if len(bp.Receivers) != len(bp.Amounts) {
		return errInconsistentAccounts
	}

	return nil
}
