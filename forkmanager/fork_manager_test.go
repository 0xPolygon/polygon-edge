package forkmanager

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	HandlerA = HandlerDesc("HA")
	HandlerB = HandlerDesc("HB")
	HandlerC = HandlerDesc("HC")
	HandlerD = HandlerDesc("HD")

	ForkA = "A"
	ForkB = "B"
	ForkC = "C"
	ForkD = "D"
	ForkE = "E"
)

func TestForkManager(t *testing.T) {
	t.Parallel()

	forkManager := GetInstance()

	forkManager.RegisterFork(ForkA)
	forkManager.RegisterFork(ForkB)
	forkManager.RegisterFork(ForkC)
	forkManager.RegisterFork(ForkD)

	assert.NoError(t, forkManager.RegisterHandler(ForkA, HandlerA, func() string { return "AAH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkC, HandlerA, func() string { return "ACH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkA, HandlerB, func() string { return "BAH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkB, HandlerB, func() string { return "BBH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkD, HandlerB, func() string { return "BDH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkC, HandlerC, func() string { return "CCH" }))

	assert.NoError(t, forkManager.ActivateFork(ForkA, 0))
	assert.NoError(t, forkManager.ActivateFork(ForkB, 100))
	assert.NoError(t, forkManager.ActivateFork(ForkC, 200))
	assert.NoError(t, forkManager.ActivateFork(ForkD, 300))

	handlersACnt := len(forkManager.handlersMap[HandlerA])
	handlersBCnt := len(forkManager.handlersMap[HandlerB])

	assert.Equal(t, 2, handlersACnt)
	assert.Equal(t, 3, handlersBCnt)

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
}

func TestForkManager_Deactivate(t *testing.T) {
	t.Parallel()

	forkManager := &forkManager{
		forkMap:     map[string]*Fork{},
		handlersMap: map[HandlerDesc][]Handler{},
	}

	forkManager.RegisterFork(ForkA)
	forkManager.RegisterFork(ForkB)

	assert.NoError(t, forkManager.RegisterHandler(ForkA, HandlerA, func() string { return "AAH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkB, HandlerA, func() string { return "ABH" }))

	assert.NoError(t, forkManager.ActivateFork(ForkA, 0))
	assert.NoError(t, forkManager.ActivateFork(ForkB, 0))

	assert.Equal(t, 2, len(forkManager.handlersMap[HandlerA]))

	assert.NoError(t, forkManager.DeactivateFork(ForkA))

	assert.Equal(t, 1, len(forkManager.handlersMap[HandlerA]))

	assert.NoError(t, forkManager.DeactivateFork(ForkB))

	assert.Equal(t, 0, len(forkManager.handlersMap[HandlerA]))
}
