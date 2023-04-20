package service

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

var aaInvokerAddress = types.StringToAddress("0x301")

func Test_AARelayerService_Start(t *testing.T) {
	t.Parallel()

	account, err := wallet.GenerateAccount()
	require.NoError(t, err)

	log := []*ethgo.Log{
		{BlockNumber: 1, Topics: []ethgo.Hash{ethgo.ZeroHash}}, {BlockNumber: 5, Topics: []ethgo.Hash{ethgo.ZeroHash}}, {BlockNumber: 8, Topics: []ethgo.Hash{ethgo.ZeroHash}},
	}
	receipt := &ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash, Logs: log, Status: 1}
	address := types.StringToAddress("8737372123")
	aaStateTx := &AAStateTransaction{
		ID: "2891",
		Tx: &AATransaction{
			Transaction: Transaction{
				From:  address,
				Nonce: 0,
			},
		},
	}
	aaStateTxBigNonce := &AAStateTransaction{
		ID: "2892",
		Tx: &AATransaction{
			Transaction: Transaction{
				From:  address,
				Nonce: 2,
			},
		},
	}
	aaStateTxLowNonce := &AAStateTransaction{
		ID: "2893",
		Tx: &AATransaction{
			Transaction: Transaction{
				From:  address,
				Nonce: 0,
			},
		},
	}

	require.NoError(t, aaStateTx.Tx.Sign(aaInvokerAddress, chainID, account.Ecdsa, nil))

	pool := new(dummyAApool)
	pool.On("Pop").Return(aaStateTx).Once()
	pool.On("Pop").Return(aaStateTxBigNonce).Once()
	pool.On("Pop").Return(aaStateTxLowNonce).Once()
	pool.On("Pop").Return((*AAStateTransaction)(nil))
	pool.On("Update", address).Return(nil)
	pool.On("Push", mock.Anything).Return(nil)

	state := new(dummyAATxState)
	state.On("Update", mock.Anything).Return(nil)

	aaTxSender := new(dummyAATxSender)
	aaTxSender.On("GetAANonce", ethgo.Address(aaInvokerAddress), ethgo.Address(address)).
		Return(uint64(0), nil).Once()
	aaTxSender.On("GetAANonce", ethgo.Address(aaInvokerAddress), ethgo.Address(address)).
		Return(uint64(1), nil).Once()
	aaTxSender.On("GetAANonce", ethgo.Address(aaInvokerAddress), ethgo.Address(address)).
		Return(uint64(1), nil).Once()
	aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
		Return(ethgo.ZeroHash, nil).Once()
	aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(receipt, nil)

	aaRelayerService := NewAARelayerService(
		aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress,
		chainID, 0, hclog.NewNullLogger(),
		WithPullTime(100*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())

	go aaRelayerService.Start(ctx)

	time.Sleep(time.Second * 1)
	cancel()
}

func Test_AARelayerService_ExecuteJob(t *testing.T) {
	t.Parallel()

	t.Run("executeJob_ok", func(t *testing.T) {
		t.Parallel()

		log := []*ethgo.Log{
			{BlockNumber: 1, Topics: []ethgo.Hash{ethgo.ZeroHash}}, {BlockNumber: 5, Topics: []ethgo.Hash{ethgo.ZeroHash}}, {BlockNumber: 8, Topics: []ethgo.Hash{ethgo.ZeroHash}},
		}
		receipt := &ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash, Logs: log, Status: 1}
		pool := new(dummyAApool)

		account, err := wallet.GenerateAccount()
		require.NoError(t, err)

		state := new(dummyAATxState)
		state.On("Update", mock.Anything).Return(nil)

		aaTxSender := new(dummyAATxSender)
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, nil).Once()
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, time.Second*3, 5).
			Return(receipt, nil)

		aaRelayerService := NewAARelayerService(
			aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress,
			chainID, 0, hclog.NewNullLogger(),
			WithPullTime(2*time.Second), WithReceiptDelay(time.Second*3), WithNumRetries(5))

		tx := getDummyTxs()[0]

		require.NoError(t, tx.Tx.Sign(aaInvokerAddress, chainID, account.Ecdsa, nil))
		require.NoError(t, aaRelayerService.executeJob(context.Background(), tx))
	})

	t.Run("executeJob_sendTransactionError", func(t *testing.T) {
		t.Parallel()

		pool := new(dummyAApool)

		account, err := wallet.GenerateAccount()
		require.NoError(t, err)

		state := new(dummyAATxState)
		state.On("Update", mock.Anything).Return(nil)

		aaTxSender := new(dummyAATxSender)
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, errors.New("not nil")).Once()
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, nil).Once()

		aaRelayerService := NewAARelayerService(
			aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress,
			chainID, 0, hclog.NewNullLogger())

		tx := getDummyTxs()[1]

		require.NoError(t, tx.Tx.Sign(aaInvokerAddress, chainID, account.Ecdsa, nil))
		require.Error(t, aaRelayerService.executeJob(context.Background(), tx))
	})

	t.Run("executeJob_WaitForReceiptError", func(t *testing.T) {
		t.Parallel()

		account, err := wallet.GenerateAccount()
		require.NoError(t, err)

		pool := new(dummyAApool)

		state := new(dummyAATxState)
		state.On("Update", mock.Anything).Return(nil)

		aaTxSender := new(dummyAATxSender)
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, nil).Once()
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, errors.New("not nil")).Once()

		aaRelayerService := NewAARelayerService(
			aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress,
			chainID, 0, hclog.NewNullLogger())

		tx := getDummyTxs()[2]

		require.NoError(t, tx.Tx.Sign(aaInvokerAddress, chainID, account.Ecdsa, nil))
		require.Error(t, aaRelayerService.executeJob(context.Background(), tx))
	})

	t.Run("executeJob_UpdateError", func(t *testing.T) {
		t.Parallel()

		pool := new(dummyAApool)
		pool.On("Push", mock.Anything).Once()

		account, err := wallet.GenerateAccount()
		require.NoError(t, err)

		state := new(dummyAATxState)
		state.On("Update", mock.Anything).Return(errors.New("not nil"))

		aaTxSender := new(dummyAATxSender)
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, errors.New("not nil")).Once()
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, nil).Once()

		aaRelayerService := NewAARelayerService(
			aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress,
			chainID, 0, hclog.NewNullLogger())

		tx := getDummyTxs()[3]

		require.NoError(t, tx.Tx.Sign(aaInvokerAddress, chainID, account.Ecdsa, nil))
		require.Error(t, aaRelayerService.executeJob(context.Background(), tx))
	})

	t.Run("executeJob_NetError", func(t *testing.T) {
		t.Parallel()

		account, err := wallet.GenerateAccount()
		require.NoError(t, err)

		state := new(dummyAATxState)
		state.On("Update", mock.Anything).Return(errors.New("not nil"))

		pool := new(dummyAApool)
		pool.On("Push", mock.Anything)

		aaTxSender := new(dummyAATxSender)
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, net.ErrClosed).Once()
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash}, nil).Once()

		aaRelayerService := NewAARelayerService(
			aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress,
			chainID, 0, hclog.NewNullLogger())

		tx := getDummyTxs()[4]

		require.NoError(t, tx.Tx.Sign(aaInvokerAddress, chainID, account.Ecdsa, nil))
		require.Error(t, aaRelayerService.executeJob(context.Background(), tx))
	})

	t.Run("executeJob_SecondUpdateError", func(t *testing.T) {
		t.Parallel()

		receipt := &ethgo.Receipt{GasUsed: 10, BlockHash: ethgo.ZeroHash, TransactionHash: ethgo.ZeroHash, Status: 1}

		account, err := wallet.GenerateAccount()
		require.NoError(t, err)

		state := new(dummyAATxState)
		state.On("Update", mock.Anything).Return(nil)
		state.On("Update", mock.Anything).Return(errors.New("not nil")).Once()

		pool := new(dummyAApool)
		pool.On("Push", mock.Anything)

		aaTxSender := new(dummyAATxSender)
		aaTxSender.On("SendTransaction", mock.Anything, mock.Anything).
			Return(ethgo.ZeroHash, nil).Once()
		aaTxSender.On("WaitForReceipt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(receipt, nil).Once()

		tx := getDummyTxs()[0]

		aaRelayerService := NewAARelayerService(
			aaTxSender, pool, state, account.Ecdsa, aaInvokerAddress,
			chainID, 0, hclog.NewNullLogger())

		require.NoError(t, tx.Tx.Sign(aaInvokerAddress, chainID, account.Ecdsa, nil))
		require.NoError(t, aaRelayerService.executeJob(context.Background(), tx))
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

func (p *dummyAApool) Update(address types.Address) {
	args := p.Called(address)
	_ = args
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
func (t *dummyAATxState) GetAllSent() ([]*AAStateTransaction, error) {
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
	ctx context.Context, hash ethgo.Hash, delay time.Duration, numRetries int) (*ethgo.Receipt, error) {
	args := d.Called(ctx, hash, delay, numRetries)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

func (d *dummyAATxSender) SendTransaction(txn []byte) (ethgo.Hash, error) {
	args := d.Called(txn)

	return args.Get(0).(ethgo.Hash), args.Error(1) //nolint:forcetypeassert
}

func (d *dummyAATxSender) GetNonce(address ethgo.Address) (uint64, error) {
	args := d.Called(address)

	return args.Get(0).(uint64), args.Error(1) //nolint:forcetypeassert
}

func (d *dummyAATxSender) GetAANonce(invokerAddress, address ethgo.Address) (uint64, error) {
	args := d.Called(invokerAddress, address)

	return args.Get(0).(uint64), args.Error(1) //nolint:forcetypeassert
}

func (d *dummyAATxSender) GetGasPrice() (uint64, error) {
	return 0, nil
}
