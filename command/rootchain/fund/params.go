package fund

import (
	"errors"
	"math/big"

	cmdhelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	addressesFlag      = "addresses"
	amountsFlag        = "amounts"
	jsonRPCFlag        = "json-rpc"
	mintStakeTokenFlag = "mint"
)

var (
	errNoAddressesProvided = errors.New("no addresses provided")
	errInconsistentLength  = errors.New("validator addresses and amounts must be equal length")
)

type fundParams struct {
	addresses          []string
	amounts            []string
	stakeTokenAddr     string
	deployerPrivateKey string
	mintStakeToken     bool
	jsonRPCAddress     string

	amountValues []*big.Int
}

func (fp *fundParams) validateFlags() error {
	if len(fp.addresses) == 0 {
		return errNoAddressesProvided
	}

	if len(fp.amounts) != len(fp.addresses) {
		return errInconsistentLength
	}

	for _, addr := range fp.addresses {
		if err := types.IsValidAddress(addr); err != nil {
			return err
		}
	}

	fp.amountValues = make([]*big.Int, len(fp.amounts))
	for i, amountRaw := range fp.amounts {
		amountValue, err := cmdhelper.ParseAmount(amountRaw)
		if err != nil {
			return err
		}

		fp.amountValues[i] = amountValue
	}

	return nil
}
