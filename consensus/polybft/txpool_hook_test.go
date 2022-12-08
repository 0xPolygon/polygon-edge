package polybft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTxpoolHook(t *testing.T) {
	txPool := new(txPoolMock)

	var found *types.Header

	txPool.On("ResetWithHeaders", mock.MatchedBy(func(i interface{}) bool {
		ph, _ := i.([]*types.Header)
		found = ph[0]

		return true
	}))

	hook := newTxpoolHook(txPool)
	hook.PostBlock(&PostBlockRequest{
		Header: &types.Header{
			Number: 10,
		},
	})

	require.Equal(t, found.Number, uint64(10))
}
