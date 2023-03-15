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
		receipt := &ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash, Logs: log, Status: 1}
		state := new(dummyAATxState)
		pool := new(dummyAApool)
		aaTxSender := new(dummyAATxSender)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(nil)
		aaRelayerService := NewAARelayerService(aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress, opts)
		tx := getDummyTxs()[0]
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, nil).Once()
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything).
			Return(receipt, nil)

		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)

		err := aaRelayerService.executeJob(context.Background(), tx)

		require.NoError(t, err)
	})
	t.Run("executeJob_sendTransactionError", func(t *testing.T) {
		t.Parallel()

		state := new(dummyAATxState)
		pool := new(dummyAApool)
		aaTxSender := new(dummyAATxSender)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(nil)
		aaRelayerService := NewAARelayerService(aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress)
		tx := getDummyTxs()[1]
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, errors.New("not nil")).Once()
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything).
			Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, nil).Once()
		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)
		err := aaRelayerService.executeJob(context.Background(), tx)
		require.Error(t, err)
	})
	t.Run("executeJob_WaitForReceiptError", func(t *testing.T) {
		t.Parallel()

		state := new(dummyAATxState)
		pool := new(dummyAApool)
		aaTxSender := new(dummyAATxSender)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(nil)
		aaRelayerService := NewAARelayerService(aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress)
		tx := getDummyTxs()[2]
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, nil).Once()
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything).
			Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, errors.New("not nil")).Once()
		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)
		err := aaRelayerService.executeJob(context.Background(), tx)
		require.Error(t, err)
	})
	t.Run("executeJob_UpdateError", func(t *testing.T) {
		t.Parallel()

		state := new(dummyAATxState)
		pool := new(dummyAApool)
		aaTxSender := new(dummyAATxSender)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(errors.New("not nil"))
		aaRelayerService := NewAARelayerService(aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress)
		tx := getDummyTxs()[3]
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, errors.New("not nil")).Once()
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything).
			Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, nil).Once()

		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)

		err := aaRelayerService.executeJob(context.Background(), tx)

		require.Error(t, err)
	})

	t.Run("executeJob_NetError", func(t *testing.T) {
		t.Parallel()

		state := new(dummyAATxState)
		pool := new(dummyAApool)
		aaTxSender := new(dummyAATxSender)
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(errors.New("not nil"))
		aaRelayerService := NewAARelayerService(aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress)
		tx := getDummyTxs()[4]
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, net.ErrClosed).Once()
		pool.On("Push", mock.Anything)
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything).
			Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, nil).Once()

		tx.Tx.MakeSignature(aaInvokerAddress, chainID, account.Ecdsa)

		err := aaRelayerService.executeJob(context.Background(), tx)

		require.Error(t, err)
	})
	t.Run("executeJob_SecondUpdateError", func(t *testing.T) {
		t.Parallel()

		state := new(dummyAATxState)
		pool := new(dummyAApool)
		aaTxSender := new(dummyAATxSender)
		receipt := &ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash, Status: 1}
		account := wallet.GenerateAccount()
		state.On("Update", mock.Anything).Return(errors.New("not nil")).Once()
		aaRelayerService := NewAARelayerService(aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress)
		tx := getDummyTxs()[0]
		state.On("Update", mock.Anything).Return(nil)
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, nil).Once()
		pool.On("Push", mock.Anything)
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything).
			Return(receipt, nil).Once()

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

type dummyAATxSender struct {
	mock.Mock

	test             *testing.T
	checkpointBlocks []uint64
}

func newDummyAATxSender(t *testing.T) *dummyAATxSender {
	t.Helper()

	return &dummyAATxSender{test: t}
}

func (d *dummyAATxSender) WaitForReceipt(
	ctx context.Context, hash ethgo.Hash, delay time.Duration) (*ethgo.Receipt, error) {
	args := d.Called(ctx, hash, delay)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

func (d *dummyAATxSender) SendTransaction(txn *ethgo.Transaction, key ethgo.Key) (ethgo.Hash, error) {
	args := d.Called()

	return args.Get(0).(ethgo.Hash), args.Error(1) //nolint:forcetypeassert
}