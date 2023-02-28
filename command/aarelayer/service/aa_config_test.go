package service

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func Test_AAConfig(t *testing.T) {
	t.Parallel()

	t.Run("EmptyAddress_AllowContractCreationTrue", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{}

		assert.False(t, config.IsValidAddress(nil))
	})

	t.Run("EmptyAddress_AllowContractCreationFalse", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{AllowContractCreation: true}

		assert.True(t, config.IsValidAddress(nil))
	})

	t.Run("InBlackList", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{DenyList: []string{"0000000000000000000000000000000000000000"}}

		assert.False(t, config.IsValidAddress(&types.ZeroAddress))
	})

	t.Run("WhiteListEmpty", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{DenyList: []string{"0000000000000000000000000000000000000000"}}

		address := types.Address{1, 2}
		assert.True(t, config.IsValidAddress(&address))
	})

	t.Run("NotInWhiteList", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{AllowList: []string{"1000000000000000000000000000000000000000"}}

		address := types.Address{1, 2}
		assert.False(t, config.IsValidAddress(&address))
	})

	t.Run("InWhiteList", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{AllowList: []string{"0101000000000000000000000000000000000000"}}

		address := types.Address{1, 1}
		assert.True(t, config.IsValidAddress(&address))
	})
}
