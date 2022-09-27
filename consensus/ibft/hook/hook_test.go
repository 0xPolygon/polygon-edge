package hook

import (
	"errors"
	"testing"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

var (
	testHeader = &types.Header{
		Number:    1,
		Miner:     []byte{},
		ExtraData: []byte{},
	}

	testBlock = &types.Block{
		Header:       testHeader,
		Transactions: []*types.Transaction{},
		Uncles:       []*types.Header{},
	}

	addr1 = types.StringToAddress("1")

	errTest = errors.New("error test")
)

func newTestHooks(
	shouldWriteTransactions ShouldWriteTransactionsFunc,
	modifyHeader ModifyHeaderFunc,
	verifyHeader VerifyHeaderFunc,
	verifyBlock VerifyBlockFunc,
	processHeaderFunc ProcessHeaderFunc,
	preCommitState PreCommitStateFunc,
	postInsertBlock PostInsertBlockFunc,
) *Hooks {
	return &Hooks{
		ShouldWriteTransactionFunc: shouldWriteTransactions,
		ModifyHeaderFunc:           modifyHeader,
		VerifyHeaderFunc:           verifyHeader,
		VerifyBlockFunc:            verifyBlock,
		ProcessHeaderFunc:          processHeaderFunc,
		PreCommitStateFunc:         preCommitState,
		PostInsertBlockFunc:        postInsertBlock,
	}
}

func TestShouldWriteTransactions(t *testing.T) {
	t.Parallel()

	t.Run("should return true if the function is not set", func(t *testing.T) {
		t.Parallel()

		hooks := newTestHooks(nil, nil, nil, nil, nil, nil, nil)

		assert.True(t, hooks.ShouldWriteTransactions(0))
	})

	t.Run("should call ShouldWriteTransactionFunc", func(t *testing.T) {
		t.Parallel()

		shouldWriteTransaction := func(x uint64) bool {
			assert.LessOrEqual(t, x, uint64(1))

			return x != 0
		}

		hooks := newTestHooks(shouldWriteTransaction, nil, nil, nil, nil, nil, nil)

		assert.False(t, hooks.ShouldWriteTransactions(0))
		assert.True(t, hooks.ShouldWriteTransactions(1))
	})
}

func TestModifyHeader(t *testing.T) {
	t.Parallel()

	t.Run("should do nothing if the function is not set", func(t *testing.T) {
		t.Parallel()

		header := testHeader.Copy()

		hooks := newTestHooks(nil, nil, nil, nil, nil, nil, nil)

		assert.Nil(t, hooks.ModifyHeader(header, addr1))
		assert.Equal(t, testHeader, header)
	})

	t.Run("should call ModifyHeader", func(t *testing.T) {
		t.Parallel()

		header := testHeader.Copy()

		modifyHeader := func(h *types.Header, proposer types.Address) error {
			assert.Equal(t, header, h)
			assert.Equal(t, addr1, proposer)

			h.Miner = proposer.Bytes()

			return nil
		}

		hooks := newTestHooks(nil, modifyHeader, nil, nil, nil, nil, nil)

		assert.Nil(t, hooks.ModifyHeader(header, addr1))
		assert.Equal(
			t,
			&types.Header{
				Number:    1,
				Miner:     addr1.Bytes(),
				ExtraData: []byte{},
			},
			header,
		)
	})
}

//nolint:dupl
func TestVerifyHeader(t *testing.T) {
	t.Parallel()

	t.Run("should return nil if the function is not set", func(t *testing.T) {
		t.Parallel()

		header := testHeader.Copy()

		hooks := newTestHooks(nil, nil, nil, nil, nil, nil, nil)

		assert.Nil(t, hooks.VerifyHeader(header))
		assert.Equal(t, testHeader, header)
	})

	t.Run("should call VerifyHeader", func(t *testing.T) {
		t.Parallel()

		header := testHeader.Copy()

		verifyHeader := func(h *types.Header) error {
			assert.Equal(t, header, h)

			return errTest
		}

		hooks := newTestHooks(nil, nil, verifyHeader, nil, nil, nil, nil)

		assert.Equal(
			t,
			errTest,
			hooks.VerifyHeader(header),
		)
		assert.Equal(
			t,
			testHeader,
			header,
		)
	})
}

//nolint:dupl
func TestVerifyBlock(t *testing.T) {
	t.Parallel()

	t.Run("should return nil if the function is not set", func(t *testing.T) {
		t.Parallel()

		block := &types.Block{
			Header:       testBlock.Header.Copy(),
			Transactions: []*types.Transaction{},
			Uncles:       []*types.Header{},
		}

		hooks := newTestHooks(nil, nil, nil, nil, nil, nil, nil)

		assert.Nil(t, hooks.VerifyBlock(testBlock))
		assert.Equal(t, testBlock, block)
	})

	t.Run("should call VerifyHeader", func(t *testing.T) {
		t.Parallel()

		block := &types.Block{
			Header:       testBlock.Header.Copy(),
			Transactions: []*types.Transaction{},
			Uncles:       []*types.Header{},
		}

		verifyBlock := func(b *types.Block) error {
			assert.Equal(t, block, b)

			return errTest
		}

		hooks := newTestHooks(nil, nil, nil, verifyBlock, nil, nil, nil)

		assert.Equal(
			t,
			errTest,
			hooks.VerifyBlock(block),
		)
		assert.Equal(
			t,
			testBlock,
			block,
		)
	})
}

//nolint:dupl
func TestProcessHeader(t *testing.T) {
	t.Parallel()

	t.Run("should do nothing if the function is not set", func(t *testing.T) {
		t.Parallel()

		header := testHeader.Copy()

		hooks := newTestHooks(nil, nil, nil, nil, nil, nil, nil)

		assert.Nil(t, hooks.ProcessHeader(header))
		assert.Equal(t, testHeader, header)
	})

	t.Run("should call ProcessHeader", func(t *testing.T) {
		t.Parallel()

		header := testHeader.Copy()

		processHeader := func(h *types.Header) error {
			assert.Equal(t, header, h)

			return errTest
		}

		hooks := newTestHooks(nil, nil, nil, nil, processHeader, nil, nil)

		assert.Equal(
			t,
			errTest,
			hooks.ProcessHeader(header),
		)
		assert.Equal(
			t,
			testHeader,
			header,
		)
	})
}

func TestPreCommitState(t *testing.T) {
	t.Parallel()

	var (
		txn = &state.Transition{}
	)

	t.Run("should do nothing if the function is not set", func(t *testing.T) {
		t.Parallel()

		header := testHeader.Copy()

		hooks := newTestHooks(nil, nil, nil, nil, nil, nil, nil)

		assert.Nil(t, hooks.PreCommitState(header, txn))
		assert.Equal(t, testHeader, header)
	})

	t.Run("should call ProcessHeader", func(t *testing.T) {
		t.Parallel()

		header := testHeader.Copy()

		preCommitState := func(h *types.Header, x *state.Transition) error {
			assert.Equal(t, header, h)
			assert.Equal(t, txn, x)

			return errTest
		}

		hooks := newTestHooks(nil, nil, nil, nil, nil, preCommitState, nil)

		assert.Equal(
			t,
			errTest,
			hooks.PreCommitState(header, txn),
		)
		assert.Equal(
			t,
			testHeader,
			header,
		)
	})
}

//nolint:dupl
func TestPostInsertBlock(t *testing.T) {
	t.Parallel()

	t.Run("should do nothing if the function is not set", func(t *testing.T) {
		t.Parallel()

		block := &types.Block{
			Header:       testBlock.Header.Copy(),
			Transactions: []*types.Transaction{},
			Uncles:       []*types.Header{},
		}

		hooks := newTestHooks(nil, nil, nil, nil, nil, nil, nil)

		assert.Nil(t, hooks.PostInsertBlock(block))
		assert.Equal(t, testBlock, block)
	})

	t.Run("should call ProcessHeader", func(t *testing.T) {
		t.Parallel()

		block := &types.Block{
			Header:       testBlock.Header.Copy(),
			Transactions: []*types.Transaction{},
			Uncles:       []*types.Header{},
		}

		postBlock := func(b *types.Block) error {
			assert.Equal(t, block, b)

			return errTest
		}

		hooks := newTestHooks(nil, nil, nil, nil, nil, nil, postBlock)

		assert.Equal(
			t,
			errTest,
			hooks.PostInsertBlock(block),
		)
		assert.Equal(
			t,
			testBlock,
			block,
		)
	})
}
