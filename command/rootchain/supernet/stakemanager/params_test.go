package stakemanager

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
)

func TestValidateFlags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		params *stakeManagerDeployParams
		errMsg string
	}{
		{
			params: &stakeManagerDeployParams{
				isTestMode:        false,
				privateKey:        rootHelper.TestAccountPrivKey,
				stakeTokenAddress: "",
			},
			errMsg: rootHelper.ErrMandatoryStakeToken.Error(),
		},
		{
			params: &stakeManagerDeployParams{
				isTestMode:        false,
				privateKey:        rootHelper.TestAccountPrivKey,
				stakeTokenAddress: "0x1B",
			},
			errMsg: "invalid stake token address is provided",
		},
	}

	for i, tc := range testCases {
		i := i
		tc := tc

		t.Run(fmt.Sprintf("case#%d", i+1), func(t *testing.T) {
			t.Parallel()

			err := tc.params.validateFlags()
			require.ErrorContains(t, err, tc.errMsg)
		})
	}
}
