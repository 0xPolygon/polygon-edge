package service

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

var aaInvokerAddress = types.StringToAddress("0x301")

func Test_AARelayerService_Start(t *testing.T) {
	t.Parallel()

	t.Run("executeJob_ok", func(t *testing.T) {
		t.Parallel()
		opts := WithPullTime(2 * time.Second)
		var log = []*ethgo.Log{
			{BlockNumber: 1, Topics: []ethgo.Hash{ethgo.ZeroHash}}, {BlockNumber: 5, Topics: []ethgo.Hash{ethgo.ZeroHash}}, {BlockNumber: 8, Topics: []ethgo.Hash{ethgo.ZeroHash}},
		}
		state := new(dummyAATxState)
		pool := new(dummyAApool)
		relayer := new(dummyTxRelayer)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(nil)
		aaRelayerService := NewAARelayerService(relayer, pool, state, account.Ecdsa, opts)
		tx := getDummyTxs()[0]
		relayer.On("SendTransactionWithoutReceipt", mock.Anything, mock.Anything).Return(ethgo.ZeroHash, nil).Once()
		relayer.On("WaitForReceipt", mock.Anything, mock.Anything).Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash, Logs: log}, nil)

		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)

		err := aaRelayerService.executeJob(context.Background(), tx)

		require.NoError(t, err)
	})
	t.Run("executeJob_sendTransactionError", func(t *testing.T) {
		t.Parallel()

		state := new(dummyAATxState)
		pool := new(dummyAApool)
		relayer := new(dummyTxRelayer)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(nil)
		aaRelayerService := NewAARelayerService(relayer, pool, state, account.Ecdsa)
		tx := getDummyTxs()[1]
		relayer.On("SendTransactionWithoutReceipt", mock.Anything, mock.Anything).Return(ethgo.ZeroHash, errors.New("not nil")).Once()
		relayer.On("WaitForReceipt", mock.Anything, mock.Anything).Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, nil).Once()
		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)
		err := aaRelayerService.executeJob(context.Background(), tx)
		require.Error(t, err)
	})
	t.Run("executeJob_WaitForReceiptError", func(t *testing.T) {
		t.Parallel()

		state := new(dummyAATxState)
		pool := new(dummyAApool)
		relayer := new(dummyTxRelayer)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(nil)
		aaRelayerService := NewAARelayerService(relayer, pool, state, account.Ecdsa)
		tx := getDummyTxs()[2]
		relayer.On("SendTransactionWithoutReceipt", mock.Anything, mock.Anything).Return(ethgo.ZeroHash, nil).Once()
		relayer.On("WaitForReceipt", mock.Anything, mock.Anything).Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, errors.New("not nil")).Once()
		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)
		err := aaRelayerService.executeJob(context.Background(), tx)
		require.Error(t, err)
	})
	t.Run("executeJob_UpdateError", func(t *testing.T) {
		t.Parallel()

		state := new(dummyAATxState)
		pool := new(dummyAApool)
		relayer := new(dummyTxRelayer)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(errors.New("not nil"))
		aaRelayerService := NewAARelayerService(relayer, pool, state, account.Ecdsa)
		tx := getDummyTxs()[3]
		relayer.On("SendTransactionWithoutReceipt", mock.Anything, mock.Anything).Return(ethgo.ZeroHash, errors.New("not nil")).Once()
		relayer.On("WaitForReceipt", mock.Anything, mock.Anything).Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, nil).Once()

		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)

		err := aaRelayerService.executeJob(context.Background(), tx)

		require.Error(t, err)
	})

	t.Run("executeJob_NetError", func(t *testing.T) {
		t.Parallel()

		state := new(dummyAATxState)
		pool := new(dummyAApool)
		relayer := new(dummyTxRelayer)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(errors.New("not nil"))
		aaRelayerService := NewAARelayerService(relayer, pool, state, account.Ecdsa)
		tx := getDummyTxs()[4]
		relayer.On("SendTransactionWithoutReceipt", mock.Anything, mock.Anything).Return(ethgo.ZeroHash, net.ErrClosed).Once()
		pool.On("Push", mock.Anything)
		relayer.On("WaitForReceipt", mock.Anything, mock.Anything).Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, nil).Once()

		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)

		err := aaRelayerService.executeJob(context.Background(), tx)

		require.Error(t, err)
	})
	t.Run("executeJob_SecondUpdateError", func(t *testing.T) {
		t.Parallel()

		state := new(dummyAATxState)
		pool := new(dummyAApool)
		relayer := new(dummyTxRelayer)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(errors.New("not nil")).Once()
		aaRelayerService := NewAARelayerService(relayer, pool, state, account.Ecdsa)
		tx := getDummyTxs()[0]
		state.On("Update", mock.Anything).Return(nil)
		relayer.On("SendTransactionWithoutReceipt", mock.Anything, mock.Anything).Return(ethgo.ZeroHash, nil).Once()
		pool.On("Push", mock.Anything)
		relayer.On("WaitForReceipt", mock.Anything, mock.Anything).Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, nil).Once()

		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)

		err := aaRelayerService.executeJob(context.Background(), tx)

		require.NoError(t, err)
	})
}

type dummyAApool struct {
	mock.Mock
}

func (p *dummyAApool) Push(stateTx *AAStateTransaction) {
	args := p.Called()
	_ = args
}

func (p *dummyAApool) Pop() *AAStateTransaction {
	args := p.Called()

	return args.Get(0).(*AAStateTransaction) //nolint:forcetypeassert
}

func (p *dummyAApool) Init(txs []*AAStateTransaction) {
	args := p.Called(txs)

	_ = args
}
func (p *dummyAApool) Len() int {
	args := p.Called()

	return args.Int(0)
}

type dummyAATxState struct {
	mock.Mock
}

func (t *dummyAATxState) Add(transaction *AATransaction) (*AAStateTransaction, error) {
	args := t.Called()

	return args.Get(0).(*AAStateTransaction), args.Error(1) //nolint:forcetypeassert
}

func (t *dummyAATxState) Get(id string) (*AAStateTransaction, error) {
	args := t.Called(id)

	return args.Get(0).(*AAStateTransaction), args.Error(1) //nolint:forcetypeassert
}

func (t *dummyAATxState) GetAllPending() ([]*AAStateTransaction, error) {
	args := t.Called()

	return args.Get(0).([]*AAStateTransaction), args.Error(1) //nolint:forcetypeassert
}
func (t *dummyAATxState) GetAllQueued() ([]*AAStateTransaction, error) {
	args := t.Called()

	return args.Get(0).([]*AAStateTransaction), args.Error(1) //nolint:forcetypeassert
}
func (t *dummyAATxState) Update(stateTx *AAStateTransaction) error {
	args := t.Called()

	if stateTx.Status == StatusFailed {
		return errors.New("Update failed")
	}

	return args.Error(0)
}

type dummyTxRelayer struct {
	mock.Mock

	test             *testing.T
	checkpointBlocks []uint64
}

func newDummyTxRelayer(t *testing.T) *dummyTxRelayer {
	t.Helper()

	return &dummyTxRelayer{test: t}
}

func (d dummyTxRelayer) Call(from ethgo.Address, to ethgo.Address, input []byte) (string, error) {
	args := d.Called(from, to, input)

	return args.String(0), args.Error(1)
}

func (d *dummyTxRelayer) SendTransaction(transaction *ethgo.Transaction, key ethgo.Key) (*ethgo.Receipt, error) {
	args := d.Called(transaction, key)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

// SendTransactionLocal sends non-signed transaction (this is only for testing purposes)
func (d *dummyTxRelayer) SendTransactionLocal(txn *ethgo.Transaction) (*ethgo.Receipt, error) {
	args := d.Called(txn)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

func (d *dummyTxRelayer) WaitForReceipt(ctx context.Context, hash ethgo.Hash) (*ethgo.Receipt, error) {
	args := d.Called(ctx, hash)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

func (d *dummyTxRelayer) SendTransactionWithoutReceipt(txn *ethgo.Transaction, key ethgo.Key) (ethgo.Hash, error) {
	args := d.Called()

	return args.Get(0).(ethgo.Hash), args.Error(1) //nolint:forcetypeassert
}
