package statesyncrelayer

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/mock"
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

	return &ethgo.Receipt{}, args.Error(1)
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

	sp := &types.StateSyncProof{
		Proof:     []types.Hash{},
		StateSync: &types.StateSyncEvent{},
	}

	input, _ := types.ExecuteStateSyncABIMethod.Encode(
		[2]interface{}{sp.Proof, sp.StateSync.ToMap()},
	)

	txn := &ethgo.Transaction{
		From:  key.Address(),
		To:    (*ethgo.Address)(&contracts.StateReceiverContract),
		Input: input,
		Gas:   types.StateTransactionGasLimit,
	}

	txRelayer.On("SendTransaction", txn, key).Return(&ethgo.Receipt{}, nil)
	r.executeStateSync(sp)

	txRelayer.AssertExpectations(t)
}

// Test sanitizeRPCEndpoint
func Test_sanitizeRPCEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			"url with port",
			"http://localhost:10001",
			"http://localhost:10001",
		},
		{
			"all interfaces with port without schema",
			"0.0.0.0:10001",
			"http://127.0.0.1:10001",
		},
		{
			"url without port",
			"http://127.0.0.1",
			"http://127.0.0.1",
		},
		{
			"empty endpoint",
			"",
			"http://127.0.0.1:8545",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := sanitizeRPCEndpoint(tt.endpoint); got != tt.want {
				t.Errorf("sanitizeRPCEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}
