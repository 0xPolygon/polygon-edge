package relayer

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

	return nil, args.Error(1)
}

func (t *txRelayerMock) SendTransactionLocal(txn *ethgo.Transaction) (*ethgo.Receipt, error) {
	args := t.Called(txn)

	return nil, args.Error(1)
}

func Test_executeStateSync(t *testing.T) {
	txRelayer := &txRelayerMock{}
	key, _ := wallet.GenerateKey()

	r := &Relayer{
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
