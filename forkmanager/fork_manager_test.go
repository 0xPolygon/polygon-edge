package forkmanager

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForkManager(t *testing.T) {
	t.Parallel()

	forkManager := GetInstance()

	forkManager.RegisterFork(ForkName("A"))
	forkManager.RegisterFork(ForkName("B"))
	forkManager.RegisterFork(ForkName("C"))
	forkManager.RegisterFork(ForkName("D"))

	assert.NoError(t, forkManager.RegisterHandler(ForkName("A"), "A", func() string { return "AAH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkName("C"), "A", func() string { return "ACH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkName("A"), "B", func() string { return "BAH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkName("B"), "B", func() string { return "BBH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkName("D"), "B", func() string { return "BDH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkName("B"), "C", func() string { return "CBH" }))

	assert.NoError(t, forkManager.ActivateFork(ForkName("A"), 0))
	assert.NoError(t, forkManager.ActivateFork(ForkName("B"), 100))
	assert.NoError(t, forkManager.ActivateFork(ForkName("C"), 200))
	assert.NoError(t, forkManager.ActivateFork(ForkName("D"), 300))

	handlersACnt := len(forkManager.handlersMap[ForkHandlerName("A")])
	handlersBCnt := len(forkManager.handlersMap[ForkHandlerName("B")])

	assert.Equal(t, 2, handlersACnt)
	assert.Equal(t, 3, handlersBCnt)

	t.Run("activate not registered fork", func(t *testing.T) {
		t.Parallel()

		assert.Error(t, forkManager.ActivateFork(ForkName("EE"), 100))
	})

	t.Run("activate already activated fork", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, forkManager.ActivateFork(ForkName("A"), 100))

		// count not changed
		assert.Equal(t, handlersACnt, len(forkManager.handlersMap[ForkHandlerName("A")]))
		assert.Equal(t, handlersBCnt, len(forkManager.handlersMap[ForkHandlerName("B")]))
	})

	t.Run("is fork enabled", func(t *testing.T) {
		t.Parallel()

		assert.True(t, forkManager.IsForkEnabled(ForkName("A"), 0))
		assert.True(t, forkManager.IsForkEnabled(ForkName("A"), 100))
		assert.True(t, forkManager.IsForkEnabled(ForkName("A"), 200))

		assert.False(t, forkManager.IsForkEnabled(ForkName("B"), 0))
		assert.True(t, forkManager.IsForkEnabled(ForkName("B"), 100))
		assert.True(t, forkManager.IsForkEnabled(ForkName("B"), 200))

		assert.False(t, forkManager.IsForkEnabled(ForkName("C"), 0))
		assert.False(t, forkManager.IsForkEnabled(ForkName("C"), 100))
		assert.True(t, forkManager.IsForkEnabled(ForkName("C"), 200))
		assert.True(t, forkManager.IsForkEnabled(ForkName("C"), 300))

		assert.False(t, forkManager.IsForkEnabled(ForkName("D"), 0))
		assert.False(t, forkManager.IsForkEnabled(ForkName("D"), 100))
		assert.False(t, forkManager.IsForkEnabled(ForkName("D"), 200))
		assert.True(t, forkManager.IsForkEnabled(ForkName("D"), 300))
		assert.True(t, forkManager.IsForkEnabled(ForkName("D"), 400))

		assert.False(t, forkManager.IsForkEnabled(ForkName("FF"), 0))
	})

	t.Run("is fork supported", func(t *testing.T) {
		t.Parallel()

		assert.True(t, forkManager.IsForkSupported(ForkName("A")))
		assert.True(t, forkManager.IsForkSupported(ForkName("B")))
		assert.True(t, forkManager.IsForkSupported(ForkName("C")))
		assert.True(t, forkManager.IsForkSupported(ForkName("D")))

		assert.False(t, forkManager.IsForkSupported(ForkName("CC")))
	})

	t.Run("get fork block", func(t *testing.T) {
		t.Parallel()

		b, err := forkManager.GetForkBlock(ForkName("A"))
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), b)

		b, err = forkManager.GetForkBlock(ForkName("B"))
		assert.NoError(t, err)
		assert.Equal(t, uint64(100), b)

		b, err = forkManager.GetForkBlock(ForkName("C"))
		assert.NoError(t, err)
		assert.Equal(t, uint64(200), b)

		b, err = forkManager.GetForkBlock(ForkName("D"))
		assert.NoError(t, err)
		assert.Equal(t, uint64(300), b)

		_, err = forkManager.GetForkBlock(ForkName("DD"))
		assert.Error(t, err)
	})

	t.Run("register handler not existing fork", func(t *testing.T) {
		t.Parallel()

		assert.Error(t, forkManager.RegisterHandler(ForkName("EEE"), ForkHandlerName("E"), func() {}))
	})

	t.Run("get handler", func(t *testing.T) {
		t.Parallel()

		for i := 0; i < 4; i++ {
			forkManager.SetCurrentBlock(uint64(i))

			assert.Equal(t, "AAH", forkManager.GetHandler("A").(func() string)()) //nolint:forcetypeassert
			assert.Equal(t, "BAH", forkManager.GetHandler("B").(func() string)()) //nolint:forcetypeassert
			assert.Nil(t, forkManager.GetHandler("C"))

			forkManager.SetCurrentBlock(uint64(100 + i))

			assert.Equal(t, "AAH", forkManager.GetHandler("A").(func() string)()) //nolint:forcetypeassert
			assert.Equal(t, "BBH", forkManager.GetHandler("B").(func() string)()) //nolint:forcetypeassert
			assert.Equal(t, "CBH", forkManager.GetHandler("C").(func() string)()) //nolint:forcetypeassert

			forkManager.SetCurrentBlock(uint64(200 + i))

			assert.Equal(t, "ACH", forkManager.GetHandler("A").(func() string)()) //nolint:forcetypeassert
			assert.Equal(t, "BBH", forkManager.GetHandler("B").(func() string)()) //nolint:forcetypeassert
			assert.Equal(t, "CBH", forkManager.GetHandler("C").(func() string)()) //nolint:forcetypeassert

			forkManager.SetCurrentBlock(uint64(300 + i))

			assert.Equal(t, "ACH", forkManager.GetHandler("A").(func() string)()) //nolint:forcetypeassert
			assert.Equal(t, "BDH", forkManager.GetHandler("B").(func() string)()) //nolint:forcetypeassert
			assert.Equal(t, "CBH", forkManager.GetHandler("C").(func() string)()) //nolint:forcetypeassert
		}

		assert.Nil(t, forkManager.GetHandler("D"))
	})
}

func TestForkManager_Deactivate(t *testing.T) {
	t.Parallel()

	forkManager := &forkManager{
		forkMap:     map[ForkName]*Fork{},
		handlersMap: map[ForkHandlerName][]ForkActiveHandler{},
	}

	forkManager.RegisterFork(ForkName("A"))
	forkManager.RegisterFork(ForkName("B"))

	assert.NoError(t, forkManager.RegisterHandler(ForkName("A"), "A", func() string { return "AAH" }))
	assert.NoError(t, forkManager.RegisterHandler(ForkName("B"), "A", func() string { return "ABH" }))

	assert.NoError(t, forkManager.ActivateFork(ForkName("A"), 0))
	assert.NoError(t, forkManager.ActivateFork(ForkName("B"), 0))

	assert.Equal(t, 2, len(forkManager.handlersMap[ForkHandlerName("A")]))

	assert.NoError(t, forkManager.DeactivateFork(ForkName("A")))

	assert.Equal(t, 1, len(forkManager.handlersMap[ForkHandlerName("A")]))

	assert.NoError(t, forkManager.DeactivateFork(ForkName("B")))

	assert.Equal(t, 0, len(forkManager.handlersMap[ForkHandlerName("A")]))
}
