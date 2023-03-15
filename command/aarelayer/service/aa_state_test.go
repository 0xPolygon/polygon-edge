package service

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_AAState_AddGetUpdate(t *testing.T) {
	t.Parallel()

	dbpath, err := os.MkdirTemp("", "aa_server_state_db")
	require.NoError(t, err)

	t.Cleanup(func() { os.RemoveAll(dbpath) })

	state, err := NewAATxState(path.Join(dbpath, "k.db"))
	require.NoError(t, err)

	stateTxs := [5]*AAStateTransaction{}

	// add 5 items
	for i := range stateTxs {
		stateTxs[i], err = state.Add(&AATransaction{Transaction: Transaction{Nonce: 10 + uint64(i)}})

		require.NoError(t, err)
		assert.True(t, len(stateTxs[i].ID) >= 5)
		assert.Equal(t, StatusPending, stateTxs[i].Status)
	}

	// retrieve
	for i, tx := range stateTxs {
		rtx, err := state.Get(tx.ID)

		require.NoError(t, err)
		assert.Equal(t, rtx.Tx.Transaction.Nonce, uint64(i)+10)
	}

	rtx, err := state.Get("not_existing_key")
	require.NoError(t, err)
	assert.Nil(t, rtx)

	// move some of txs to different bucket
	stateTxs[0].Status = StatusQueued
	require.NoError(t, state.Update(stateTxs[0]))

	stateTxs[1].Status = StatusFailed
	require.NoError(t, state.Update(stateTxs[0]))

	stateTxs[2].Status = StatusCompleted
	require.NoError(t, state.Update(stateTxs[0]))

	// retrieve should still work
	for i, tx := range stateTxs {
		rtx, err := state.Get(tx.ID)

		require.NoError(t, err)
		assert.Equal(t, rtx.Tx.Transaction.Nonce, uint64(i)+10)
	}
}

func Test_AAState_UpdateGetQueuedGetPending(t *testing.T) {
	t.Parallel()

	dbpath, err := os.MkdirTemp("", "aa_server_state_db")
	require.NoError(t, err)

	t.Cleanup(func() { os.RemoveAll(dbpath) })

	state, err := NewAATxState(path.Join(dbpath, "k.db"))
	require.NoError(t, err)

	stateTxs := [8]*AAStateTransaction{}

	// add 8 pending items
	for i := range stateTxs {
		stateTxs[i], err = state.Add(&AATransaction{Transaction: Transaction{Nonce: 10 + uint64(i)}})

		require.NoError(t, err)
		assert.True(t, len(stateTxs[i].ID) >= 5)
		assert.Equal(t, StatusPending, stateTxs[i].Status)
	}

	// update second and third to queued
	for i := 1; i <= 2; i++ {
		stateTxs[i].Status = StatusQueued
		require.NoError(t, state.Update(stateTxs[i]))
	}

	// update forth to completed
	stateTxs[3].Status = StatusCompleted
	require.NoError(t, state.Update(stateTxs[3]))

	// update sixt to failed
	stateTxs[5].Status = StatusFailed
	require.NoError(t, state.Update(stateTxs[5]))

	// retrieve
	txsQueued, err1 := state.GetAllQueued()
	txsPending, err2 := state.GetAllPending()

	assert.NoError(t, err1)
	assert.Len(t, txsPending, 4)
	assert.NoError(t, err2)
	assert.Len(t, txsQueued, 2)

	// update second to completed
	stateTxs[1].Status = StatusCompleted
	require.NoError(t, state.Update(stateTxs[1]))

	// retrieve again
	txsQueued, err1 = state.GetAllQueued()
	txsPending, err2 = state.GetAllPending()

	assert.NoError(t, err1)
	assert.Len(t, txsPending, 4)
	assert.NoError(t, err2)
	assert.Len(t, txsQueued, 1)

	// update third to failed
	stateTxs[2].Status = StatusFailed
	require.NoError(t, state.Update(stateTxs[2]))

	// retrieve again
	txsQueued, err1 = state.GetAllQueued()
	txsPending, err2 = state.GetAllPending()

	assert.NoError(t, err1)
	assert.Len(t, txsPending, 4)
	assert.NoError(t, err2)
	assert.Len(t, txsQueued, 0)

	// queue all pending
	for i := range txsPending {
		txsPending[i].Status = StatusQueued

		require.NoError(t, state.Update(txsPending[i]))
	}

	// retrieve again
	txsQueued, err1 = state.GetAllQueued()
	txsPending, err2 = state.GetAllPending()

	assert.NoError(t, err1)
	assert.Len(t, txsPending, 0)
	assert.NoError(t, err2)
	assert.Len(t, txsQueued, 4)
}
