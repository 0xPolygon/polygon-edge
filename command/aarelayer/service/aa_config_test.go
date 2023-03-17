package service

import (
	"embed"
	"encoding/json"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed config/*
var configFile embed.FS

func Test_AAConfig(t *testing.T) {
	t.Parallel()

	t.Run("InBlackList", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{DenyList: []string{"0000000000000000000000000000000000000000"}}

		assert.False(t, config.IsAddressAllowed(types.ZeroAddress))
	})

	t.Run("WhiteListEmpty", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{DenyList: []string{"0000000000000000000000000000000000000000"}}

		address := types.Address{1, 2}
		assert.True(t, config.IsAddressAllowed(address))
	})

	t.Run("NotInWhiteList", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{AllowList: []string{"1000000000000000000000000000000000000000"}}

		address := types.Address{1, 2}
		assert.False(t, config.IsAddressAllowed(address))
	})

	t.Run("InWhiteList", func(t *testing.T) {
		t.Parallel()

		config := AAConfig{AllowList: []string{"0101000000000000000000000000000000000000"}}

		address := types.Address{1, 1}
		assert.True(t, config.IsAddressAllowed(address))
	})

	t.Run("JSONConfig", func(t *testing.T) {
		t.Parallel()

		bytes, err := configFile.ReadFile("config/config.json")
		require.NoError(t, err)

		config := &AAConfig{}
		require.NoError(t, json.Unmarshal(bytes, &config))

		assert.True(t, config.AllowContractCreation)

		address1 := types.StringToAddress("0x71C7656EC7ab88b098defB751B7401B5f6d8976F")
		address2 := types.StringToAddress("0x71C7656EC7ab88b098defB751B7401B5f6d8976C")

		assert.True(t, config.IsAddressAllowed(address1))
		assert.False(t, config.IsAddressAllowed(address2))
	})
}
