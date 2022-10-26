package jsonrpc

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
)

type latestHeaderGetter interface {
	Header() *types.Header
}

func GetNumericBlockNumber(number BlockNumber, headerGetter latestHeaderGetter) (uint64, error) {
	switch number {
	case LatestBlockNumber:
		return headerGetter.Header().Number, nil

	case EarliestBlockNumber:
		return 0, nil

	case PendingBlockNumber:
		return 0, fmt.Errorf("fetching the pending header is not supported")

	default:
		if number < 0 {
			return 0, fmt.Errorf("invalid argument 0: block number larger than int64")
		}

		return uint64(number), nil
	}
}