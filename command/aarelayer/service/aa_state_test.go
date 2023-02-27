package service

import (
	"errors"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_AAState_AddAndGet(t *testing.T) {
	t.Parallel()

	dbpath, err := os.MkdirTemp("/tmp", "aa_server_state_db")
	require.NoError(t, err)

	t.Cleanup(func() { os.RemoveAll(dbpath) })

	state, err := NewAATxState(path.Join(dbpath, "k.db"))
	require.NoError(t, err)

	key := [3]string{"", "", ""}

	// add 3 items
	for i := 0; i < len(key); i++ {
		rtx, err := state.Add(&AATransaction{Transaction: Transaction{Nonce: 10 + uint64(i)}})

		require.NoError(t, err)
		assert.True(t, len(rtx.ID) >= 5)
		assert.Equal(t, StatusPending, rtx.Status)

		key[i] = rtx.ID
	}

	// retrieve
	for i, k := range key {
		rtx, err := state.Get(k)

		require.NoError(t, err)
		assert.Equal(t, rtx.Tx.Transaction.Nonce, uint64(i)+10)
	}

	rtx, err := state.Get("not_existing_key")
	require.NoError(t, err)
	assert.Nil(t, rtx)
}

func Test_AAState_GetPending(t *testing.T) {
	t.Parallel()

	dbpath, err := os.MkdirTemp("/tmp", "aa_server_state_db")
	require.NoError(t, err)

	t.Cleanup(func() { os.RemoveAll(dbpath) })

	state, err := NewAATxState(path.Join(dbpath, "k.db"))
	require.NoError(t, err)

	// add 3 items
	for i := 0; i < 3; i++ {
		rtx, err := state.Add(&AATransaction{Transaction: Transaction{Nonce: 10 + uint64(i)}})

		require.NoError(t, err)
		assert.True(t, len(rtx.ID) >= 5)
		assert.Equal(t, StatusPending, rtx.Status)
	}

	// retrieve
	txs, err := state.GetAllPending()

	require.NoError(t, err)
	assert.Len(t, txs, 3)
}

func Test_AAState_Update(t *testing.T) {
	t.Parallel()

	dbpath, err := os.MkdirTemp("/tmp", "aa_server_state_db")
	require.NoError(t, err)

	t.Cleanup(func() { os.RemoveAll(dbpath) })

	state, err := NewAATxState(path.Join(dbpath, "k.db"))
	require.NoError(t, err)

	key := [3]string{"", "", ""}

	// add 3 items
	for i := 0; i < 3; i++ {
		rtx, err := state.Add(&AATransaction{Transaction: Transaction{Nonce: 10 + uint64(i)}})

		require.NoError(t, err)
		assert.True(t, len(rtx.ID) >= 5)
		assert.Equal(t, StatusPending, rtx.Status)

		key[i] = rtx.ID
	}

	require.NoError(t, state.Update(key[2], func(tx *AAStateTransaction) error {
		tx.Gas = 800

		return nil
	}))

	require.NoError(t, state.Update(key[1], func(tx *AAStateTransaction) error {
		tx.Status = StatusCompleted

		return nil
	}))

	require.Error(t, state.Update("NotExist", func(tx *AAStateTransaction) error {
		tx.Status = StatusCompleted

		return nil
	}))

	require.ErrorContains(t, state.Update(key[1], func(tx *AAStateTransaction) error {
		tx.Status = StatusCompleted

		return errors.New("dummy error")
	}), "dummy error")

	rtx, err := state.Get(key[2])

	require.NoError(t, err)
	assert.Equal(t, uint64(800), rtx.Gas)

	rtx, err = state.Get(key[1])

	require.NoError(t, err)
	assert.Equal(t, rtx.Status, StatusCompleted)
}
