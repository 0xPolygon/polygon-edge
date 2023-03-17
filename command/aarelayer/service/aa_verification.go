package service

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
)

type ValidationFunc func(*AATransaction) error

type AAVerification interface {
	Validate(*AATransaction) error
}

var _ AAVerification = (*aaVerification)(nil)

type aaVerification struct {
	validationFn   ValidationFunc
	config         *AAConfig
	chainID        int64
	invokerAddress types.Address
}

func NewAAVerification(
	config *AAConfig, invokerAddress types.Address, chainID int64, validationFn ValidationFunc) *aaVerification {
	return &aaVerification{
		validationFn:   validationFn,
		config:         config,
		chainID:        chainID,
		invokerAddress: invokerAddress,
	}
}

func (p *aaVerification) Validate(tx *AATransaction) error {
	if tx == nil {
		return errors.New("tx is not valid")
	}

	if len(tx.Transaction.Payload) == 0 {
		return fmt.Errorf("tx from %s does not have any payload", tx.Transaction.From)
	}

	for _, payload := range tx.Transaction.Payload {
		if payload.GasLimit == nil {
			return fmt.Errorf("tx has invalid payload - gas limit not specified: %s", tx.Transaction.From)
		}

		if payload.Value == nil {
			return fmt.Errorf("tx has invalid payload - value not specified: %s", tx.Transaction.From)
		}

		if payload.To == nil && !p.config.AllowContractCreation {
			return fmt.Errorf("tx from %s has contract creation payload", tx.Transaction.From)
		}
	}

	if !p.config.IsAddressAllowed(tx.Transaction.From) {
		return fmt.Errorf("tx has from which is not allowed: %s", tx.Transaction.From)
	}

	// TODO: full validation will be implemented in another PR/task
	if !tx.Transaction.IsFromValid(p.invokerAddress, p.chainID, tx.Signature) {
		return fmt.Errorf("tx has invalid from: %s", tx.Transaction.From)
	}

	if p.validationFn != nil {
		return p.validationFn(tx)
	}

	return nil
}
