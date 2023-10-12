package forkmanager

import (
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	HandlerA = "HA"
	HandlerB = "HB"
	HandlerC = "HC"
	HandlerD = "HD"

	ForkA = "A"
	ForkB = "B"
	ForkC = "C"
	ForkD = "D"
	ForkE = "E"
)

func TestForkManager(t *testing.T) {
	t.Parallel()

	es1, es2, es3 := uint64(100), uint64(300), uint64(200)
	mss1 := uint64(10002)
	bt1, bt2 := common.Duration{Duration: time.Second * 5}, common.Duration{Duration: time.Second * 12}

	forkManager := GetInstance()

	forkManager.RegisterFork(ForkA, &ForkParams{EpochSize: &es1, BlockTime: &bt1})
	forkManager.RegisterFork(ForkB, &ForkParams{EpochSize: &es2, MaxValidatorSetSize: &mss1})
	forkManager.RegisterFork(ForkC, nil)
	forkManager.RegisterFork(ForkD, &ForkParams{EpochSize: &es3, BlockTime: &bt2})

	assert.NoError(t, forkManager.RegisterHandler(ForkA, HandlerA, func() string { return "AAH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkC, HandlerA, func() string { return "ACH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkA, HandlerB, func() string { return "BAH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkB, HandlerB, func() string { return "BBH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkD, HandlerB, func() string { return "BDH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkC, HandlerC, func() string { return "CCH" }))

	assert.NoError(t, forkManager.ActivateFork(ForkD, 300))
	assert.NoError(t, forkManager.ActivateFork(ForkA, 0))
	assert.NoError(t, forkManager.ActivateFork(ForkC, 200))
	assert.NoError(t, forkManager.ActivateFork(ForkB, 100))

	handlersACnt := len(forkManager.handlersMap[HandlerA])
	handlersBCnt := len(forkManager.handlersMap[HandlerB])

	assert.Equal(t, 2, handlersACnt)
	assert.Equal(t, 3, handlersBCnt)

	// double deactivate should be same as single deactivate
	assert.NoError(t, forkManager.DeactivateFork(ForkA))
	assert.NoError(t, forkManager.DeactivateFork(ForkA))
	assert.Equal(t, 1, len(forkManager.handlersMap[HandlerA]))
	assert.Equal(t, 2, len(forkManager.handlersMap[HandlerB]))

	// activate fork again
	assert.NoError(t, forkManager.ActivateFork(ForkA, 0))
	assert.Equal(t, handlersACnt, len(forkManager.handlersMap[HandlerA]))
	assert.Equal(t, handlersBCnt, len(forkManager.handlersMap[HandlerB]))

	t.Run("activate not registered fork", func(t *testing.T) {
		t.Parallel()

		assert.Error(t, forkManager.ActivateFork(ForkE, 100))
	})

	t.Run("activate already activated fork", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, forkManager.ActivateFork(ForkA, 100))

		// count not changed
		assert.Equal(t, handlersACnt, len(forkManager.handlersMap[HandlerA]))
		assert.Equal(t, handlersBCnt, len(forkManager.handlersMap[HandlerB]))
	})

	t.Run("deactivate not registered fork", func(t *testing.T) {
		t.Parallel()

		assert.Error(t, forkManager.DeactivateFork(ForkE))
	})

	t.Run("is fork enabled", func(t *testing.T) {
		t.Parallel()

		assert.True(t, forkManager.IsForkEnabled(ForkA, 0))
		assert.True(t, forkManager.IsForkEnabled(ForkA, 100))
		assert.True(t, forkManager.IsForkEnabled(ForkA, 200))

		assert.False(t, forkManager.IsForkEnabled(ForkB, 0))
		assert.True(t, forkManager.IsForkEnabled(ForkB, 100))
		assert.True(t, forkManager.IsForkEnabled(ForkB, 200))

		assert.False(t, forkManager.IsForkEnabled(ForkC, 0))
		assert.False(t, forkManager.IsForkEnabled(ForkC, 100))
		assert.True(t, forkManager.IsForkEnabled(ForkC, 200))
		assert.True(t, forkManager.IsForkEnabled(ForkC, 300))

		assert.False(t, forkManager.IsForkEnabled(ForkD, 0))
		assert.False(t, forkManager.IsForkEnabled(ForkD, 100))
		assert.False(t, forkManager.IsForkEnabled(ForkD, 200))
		assert.True(t, forkManager.IsForkEnabled(ForkD, 300))
		assert.True(t, forkManager.IsForkEnabled(ForkD, 400))

		assert.False(t, forkManager.IsForkEnabled(ForkE, 0))
	})

	t.Run("is fork supported", func(t *testing.T) {
		t.Parallel()

		assert.True(t, forkManager.IsForkRegistered(ForkA))
		assert.True(t, forkManager.IsForkRegistered(ForkB))
		assert.True(t, forkManager.IsForkRegistered(ForkC))
		assert.True(t, forkManager.IsForkRegistered(ForkD))

		assert.False(t, forkManager.IsForkRegistered(ForkE))
	})

	t.Run("get fork block", func(t *testing.T) {
		t.Parallel()

		b, err := forkManager.GetForkBlock(ForkA)
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), b)

		b, err = forkManager.GetForkBlock(ForkB)
		assert.NoError(t, err)
		assert.Equal(t, uint64(100), b)

		b, err = forkManager.GetForkBlock(ForkC)
		assert.NoError(t, err)
		assert.Equal(t, uint64(200), b)

		b, err = forkManager.GetForkBlock(ForkD)
		assert.NoError(t, err)
		assert.Equal(t, uint64(300), b)

		_, err = forkManager.GetForkBlock(ForkE)
		assert.Error(t, err)
	})

	t.Run("register handler not existing fork", func(t *testing.T) {
		t.Parallel()

		assert.Error(t, forkManager.RegisterHandler(ForkE, HandlerD, func() {}))
	})

	t.Run("get handler", func(t *testing.T) {
		t.Parallel()

		execute := func(name HandlerDesc, block uint64) string {
			//nolint:forcetypeassert
			return forkManager.GetHandler(name, block).(func() string)()
		}

		for i := uint64(0); i < uint64(4); i++ {
			assert.Equal(t, "AAH", execute(HandlerA, i))
			assert.Equal(t, "BAH", execute(HandlerB, i))
			assert.Nil(t, forkManager.GetHandler(HandlerC, i))

			assert.Equal(t, "AAH", execute(HandlerA, 100+i))
			assert.Equal(t, "BBH", execute(HandlerB, 100+i))
			assert.Nil(t, forkManager.GetHandler(HandlerC, 100+i))

			assert.Equal(t, "ACH", execute(HandlerA, 200+i))
			assert.Equal(t, "BBH", execute(HandlerB, 200+i))
			assert.Equal(t, "CCH", execute(HandlerC, 200+i))

			assert.Equal(t, "ACH", execute(HandlerA, 300+i))
			assert.Equal(t, "BDH", execute(HandlerB, 300+i))
			assert.Equal(t, "CCH", execute(HandlerC, 300+i))
		}

		assert.Nil(t, forkManager.GetHandler(HandlerD, 0))
	})

	t.Run("get params", func(t *testing.T) {
		t.Parallel()

		getParams := func(blockNum uint64) *ForkParams {
			params := forkManager.GetParams(blockNum)
			require.NotNil(t, params)

			return params
		}

		for i := uint64(0); i < uint64(4); i++ {
			assert.Equal(t, es1, *getParams(i).EpochSize)
			assert.Equal(t, es2, *getParams(i + 100).EpochSize)
			assert.Equal(t, es2, *getParams(i + 200).EpochSize)
			assert.Equal(t, es3, *getParams(i + 300).EpochSize)

			assert.Nil(t, getParams(i).MaxValidatorSetSize)
			assert.Equal(t, mss1, *getParams(i + 100).MaxValidatorSetSize)
			assert.Equal(t, mss1, *getParams(i + 200).MaxValidatorSetSize)
			assert.Equal(t, mss1, *getParams(i + 300).MaxValidatorSetSize)

			assert.Equal(t, bt1, *getParams(i).BlockTime)
			assert.Equal(t, bt1, *getParams(i + 100).BlockTime)
			assert.Equal(t, bt1, *getParams(i + 200).BlockTime)
			assert.Equal(t, bt2, *getParams(i + 300).BlockTime)
		}
	})
}

func TestForkManager_Deactivate(t *testing.T) {
	t.Parallel()

	forkManager := &forkManager{
		forkMap:     map[string]*Fork{},
		handlersMap: map[HandlerDesc][]forkHandler{},
	}
	mvs1, mvs2 := uint64(1), uint64(2)

	forkManager.RegisterFork(ForkA, &ForkParams{MaxValidatorSetSize: &mvs1})
	forkManager.RegisterFork(ForkB, &ForkParams{MaxValidatorSetSize: &mvs2})
	forkManager.RegisterFork(ForkC, &ForkParams{})

	assert.NoError(t, forkManager.RegisterHandler(ForkA, HandlerA, func() string { return "AAH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkB, HandlerA, func() string { return "ABH" }))

	assert.NoError(t, forkManager.ActivateFork(ForkB, 10))
	assert.NoError(t, forkManager.ActivateFork(ForkC, 20))
	assert.NoError(t, forkManager.ActivateFork(ForkA, 0))

	assert.Equal(t, 2, len(forkManager.handlersMap[HandlerA]))
	assert.Equal(t, 3, len(forkManager.params))

	params := forkManager.GetParams(30)
	require.NotNil(t, params)

	assert.Equal(t, mvs2, *params.MaxValidatorSetSize)

	assert.NoError(t, forkManager.DeactivateFork(ForkA))

	assert.Equal(t, 1, len(forkManager.handlersMap[HandlerA]))
	assert.Equal(t, 2, len(forkManager.params))
	assert.Nil(t, forkManager.GetParams(0))

	assert.NoError(t, forkManager.DeactivateFork(ForkC))

	assert.Equal(t, 1, len(forkManager.handlersMap[HandlerA]))
	assert.Equal(t, 1, len(forkManager.params))

	assert.NoError(t, forkManager.DeactivateFork(ForkB))

	assert.Equal(t, 0, len(forkManager.handlersMap[HandlerA]))
	assert.Equal(t, 0, len(forkManager.params))
}

func TestForkManager_HandlerReplacement(t *testing.T) {
	t.Parallel()

	forkManager := &forkManager{
		forkMap:     map[string]*Fork{},
		handlersMap: map[HandlerDesc][]forkHandler{},
	}

	execute := func(name HandlerDesc, block uint64) string {
		//nolint:forcetypeassert
		return forkManager.GetHandler(name, block).(func() string)()
	}

	forkManager.RegisterFork(ForkA, (*ForkParams)(nil))
	forkManager.RegisterFork(ForkB, nil)
	forkManager.RegisterFork(ForkC, (*ForkParams)(nil))
	forkManager.RegisterFork(ForkD, nil)
	forkManager.RegisterFork(ForkE, nil)

	assert.NoError(t, forkManager.RegisterHandler(ForkA, HandlerA, func() string { return "NNN" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkB, HandlerA, func() string { return "ABH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkC, HandlerA, func() string { return "ACH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkD, HandlerA, func() string { return "ADH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkE, HandlerA, func() string { return "AEH" }))

	assert.NoError(t, forkManager.ActivateFork(ForkA, 0))
	assert.NoError(t, forkManager.ActivateFork(ForkB, 10))
	assert.NoError(t, forkManager.ActivateFork(ForkC, 0))
	assert.NoError(t, forkManager.ActivateFork(ForkD, 10))
	assert.NoError(t, forkManager.ActivateFork(ForkE, 0))

	for i := uint64(0); i < uint64(4); i++ {
		assert.Equal(t, "AEH", execute(HandlerA, i))
		assert.Equal(t, "ADH", execute(HandlerA, i+10))
	}
}

func TestForkManager_HandlerPrecedence(t *testing.T) {
	t.Parallel()

	forkManager := &forkManager{
		forkMap:     map[string]*Fork{},
		handlersMap: map[HandlerDesc][]forkHandler{},
	}

	execute := func(name HandlerDesc, block uint64) string {
		//nolint:forcetypeassert
		return forkManager.GetHandler(name, block).(func() string)()
	}

	forkManager.RegisterFork(ForkA, nil)
	forkManager.RegisterFork(ForkB, nil)
	forkManager.RegisterFork(ForkC, nil)
	forkManager.RegisterFork(ForkD, nil)
	forkManager.RegisterFork(ForkE, nil)

	assert.NoError(t, forkManager.RegisterHandler(ForkA, HandlerA, func() string { return "A" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkB, HandlerA, func() string { return "B" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkC, HandlerA, func() string { return "C" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkD, HandlerA, func() string { return "D" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkE, HandlerA, func() string { return "E" }))

	assert.NoError(t, forkManager.ActivateFork(ForkE, 10))
	assert.NoError(t, forkManager.ActivateFork(ForkC, 2))
	assert.NoError(t, forkManager.ActivateFork(ForkA, 2))
	assert.NoError(t, forkManager.ActivateFork(ForkB, 2))
	assert.NoError(t, forkManager.ActivateFork(ForkD, 2))

	assert.Equal(t, "D", execute(HandlerA, 2))
	assert.NoError(t, forkManager.DeactivateFork(ForkD))
	assert.Equal(t, "C", execute(HandlerA, 2))
	assert.NoError(t, forkManager.DeactivateFork(ForkC))
	assert.Equal(t, "B", execute(HandlerA, 2))
	assert.NoError(t, forkManager.DeactivateFork(ForkB))
	assert.Equal(t, "A", execute(HandlerA, 3))
	assert.NoError(t, forkManager.DeactivateFork(ForkA))
	assert.Nil(t, forkManager.GetHandler(HandlerA, 0))
	assert.Equal(t, "E", execute(HandlerA, 11))

	assert.NoError(t, forkManager.ActivateFork(ForkA, 0))
	assert.NoError(t, forkManager.ActivateFork(ForkB, 0))
	assert.NoError(t, forkManager.ActivateFork(ForkC, 0))

	assert.Equal(t, "C", execute(HandlerA, 2))
	assert.NoError(t, forkManager.DeactivateFork(ForkC))
	assert.Equal(t, "B", execute(HandlerA, 1))
	assert.NoError(t, forkManager.DeactivateFork(ForkB))
	assert.Equal(t, "A", execute(HandlerA, 0))
	assert.NoError(t, forkManager.DeactivateFork(ForkA))

	assert.NoError(t, forkManager.ActivateFork(ForkB, 0))
	assert.NoError(t, forkManager.ActivateFork(ForkC, 0))
	assert.Equal(t, "C", execute(HandlerA, 0))
	assert.NoError(t, forkManager.DeactivateFork(ForkC))
	assert.Equal(t, "B", execute(HandlerA, 0))
	assert.NoError(t, forkManager.DeactivateFork(ForkB))
}
