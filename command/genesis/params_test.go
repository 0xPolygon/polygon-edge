package genesis

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/types"
)

func Test_extractNativeTokenMetadata(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		rawConfig   string
		expectedCfg *polybft.TokenConfig
		expectErr   bool
	}{
		{
			name:      "default token config",
			rawConfig: "",
			expectedCfg: &polybft.TokenConfig{
				Name:       defaultNativeTokenName,
				Symbol:     defaultNativeTokenSymbol,
				Decimals:   defaultNativeTokenDecimals,
				IsMintable: false,
				Owner:      types.ZeroAddress,
			},
			expectErr: false,
		},
		{
			name:      "not enough params provided",
			rawConfig: "Test:TST:18",
			expectErr: true,
		},
		{
			name:      "empty name provided",
			rawConfig: ":TST:18:false",
			expectErr: true,
		},
		{
			name:      "empty symbol provided",
			rawConfig: "Test::18:false",
			expectErr: true,
		},
		{
			name:      "invalid decimals number provided",
			rawConfig: "Test:TST:9999999999999999999999999999999999999999999999999999999999:false",
			expectErr: true,
		},
		{
			name:      "invalid mintable flag provided",
			rawConfig: "Test:TST:18:bar",
			expectErr: true,
		},
		{
			name:      "mintable token not enough params provided",
			rawConfig: "Test:TST:18:true",
			expectErr: true,
		},
		{
			name:      "non-mintable valid config",
			rawConfig: "MyToken:MTK:9:false",
			expectedCfg: &polybft.TokenConfig{
				Name:       "MyToken",
				Symbol:     "MTK",
				Decimals:   9,
				IsMintable: false,
				Owner:      types.ZeroAddress,
			},
			expectErr: false,
		},
		{
			name:      "non-mintable token config, owner provided but ignored",
			rawConfig: "MyToken:MTK:9:false:0x123456789",
			expectedCfg: &polybft.TokenConfig{
				Name:       "MyToken",
				Symbol:     "MTK",
				Decimals:   9,
				IsMintable: false,
				Owner:      types.ZeroAddress,
			},
			expectErr: false,
		},
		{
			name:      "mintable token valid config",
			rawConfig: "MyMintToken:MMTK:9:true:0x123456789",
			expectedCfg: &polybft.TokenConfig{
				Name:       "MyMintToken",
				Symbol:     "MMTK",
				Decimals:   9,
				IsMintable: true,
				Owner:      types.StringToAddress("0x123456789"),
			},
			expectErr: false,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			p := &genesisParams{nativeTokenConfigRaw: c.rawConfig}
			err := p.extractNativeTokenMetadata()

			if c.expectErr {
				require.Error(t, err)
				require.Nil(t, p.nativeTokenConfig)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expectedCfg, p.nativeTokenConfig)
			}
		})
	}
}
