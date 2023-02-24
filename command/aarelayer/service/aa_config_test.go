package service

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func Test_AAConfig(t *testing.T) {
	t.Parallel()

	t.Run("EmptyAddress", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{}

		assert.True(t, config.IsValidAddress(nil))
	})

	t.Run("InBlackList", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{Blacklist: []string{"0000000000000000000000000000000000000000"}}

		assert.False(t, config.IsValidAddress(&types.ZeroAddress))
	})

	t.Run("WhiteListEmpty", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{Blacklist: []string{"0000000000000000000000000000000000000000"}}

		address := types.Address{1, 2}
		assert.True(t, config.IsValidAddress(&address))
	})

	t.Run("NotInWhiteList", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{Whitelist: []string{"1000000000000000000000000000000000000000"}}

		address := types.Address{1, 2}
		assert.False(t, config.IsValidAddress(&address))
	})

	t.Run("InWhiteList", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{Whitelist: []string{"0101000000000000000000000000000000000000"}}

		address := types.Address{1, 1}
		assert.True(t, config.IsValidAddress(&address))
	})
}
