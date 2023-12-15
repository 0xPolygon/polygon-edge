package fund

import (
	"errors"
	"math/big"

	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
	cmdhelper "github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	jsonRPCFlag        = "json-rpc"
	mintStakeTokenFlag = "mint"
)

var (
	errNoAddressesProvided = errors.New("no addresses provided")
	errInconsistentLength  = errors.New("addresses and amounts must be equal length")
)

type fundParams struct {
	rawAddresses       []string
	amounts            []string
	deployerPrivateKey string
	jsonRPCAddress     string

	amountValues []*big.Int
	addresses    []types.Address
}

func (fp *fundParams) validateFlags() error {
	if len(fp.rawAddresses) == 0 {
		return bridgeHelper.ErrNoAddressesProvided
	}

	if len(fp.amounts) != len(fp.rawAddresses) {
		return bridgeHelper.ErrInconsistentLength
	}

	fp.addresses = make([]types.Address, 0, len(fp.rawAddresses))
	for _, rawAddr := range fp.rawAddresses {
		addr, err := types.IsValidAddress(rawAddr, true)
		if err != nil {
			return err
		}

		fp.addresses = append(fp.addresses, addr)
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
