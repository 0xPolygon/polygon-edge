package statesyncrelayer

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

type txRelayerMock struct {
	mock.Mock
}

func (t *txRelayerMock) Call(from ethgo.Address, to ethgo.Address, input []byte) (string, error) {
	args := t.Called(from, to, input)

	return args.String(0), args.Error(1)
}

func (t *txRelayerMock) SendTransaction(txn *ethgo.Transaction, key ethgo.Key) (*ethgo.Receipt, error) {
	args := t.Called(txn, key)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

func (t *txRelayerMock) SendTransactionLocal(txn *ethgo.Transaction) (*ethgo.Receipt, error) {
	args := t.Called(txn)

	return nil, args.Error(1)
}

func Test_executeStateSync(t *testing.T) {
	t.Parallel()

	txRelayer := &txRelayerMock{}
	key, _ := wallet.GenerateKey()

	r := &StateSyncRelayer{
		txRelayer: txRelayer,
		key:       key,
	}

	txRelayer.On("SendTransaction", mock.Anything, mock.Anything).
		Return(&ethgo.Receipt{Status: uint64(types.ReceiptSuccess)}, nil).Once()

	proof := &types.Proof{
		Data: []types.Hash{},
		Metadata: map[string]interface{}{
			"StateSync": map[string]interface{}{
				"ID":       big.NewInt(1),
				"Sender":   types.ZeroAddress,
				"Receiver": types.ZeroAddress,
				"Data":     []byte{},
			},
		},
	}

	require.NoError(t, r.executeStateSync(proof))

	txRelayer.AssertExpectations(t)
}

func TestStateSyncRelayer_Stop(t *testing.T) {
	t.Parallel()

	key, err := wallet.GenerateKey()
	require.NoError(t, err)

	r := NewRelayer("test-chain-1", "http://127.0.0.1:8545", ethgo.Address(contracts.StateReceiverContract), 0, hclog.NewNullLogger(), key)

	require.NotPanics(t, func() { r.Stop() })
}
