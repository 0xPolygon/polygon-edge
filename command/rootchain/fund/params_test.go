package fund

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

func Test_validateFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		buildParamsFn func() *fundParams
		err           string
	}{
		{
			// no addresses provided
			buildParamsFn: func() *fundParams {
				return &fundParams{}
			},
			err: errNoAddressesProvided.Error(),
		},
		{
			// inconsistent length (addresses vs amounts)
			buildParamsFn: func() *fundParams {
				return &fundParams{
					addresses: []string{"0x1", "0x2"},
					amounts:   []string{"10"},
				}
			},
			err: errInconsistentLength.Error(),
		},
		{
			// address contains invalid characters
			buildParamsFn: func() *fundParams {
				return &fundParams{
					addresses: []string{"0x10", "0x20"},
					amounts:   []string{"10", "20"},
				}
			},
			err: "address \x10 has invalid length",
		},
		{
			// stake token address omitted
			buildParamsFn: func() *fundParams {
				return &fundParams{
					mintStakeToken: true,
					addresses: []string{
						types.StringToAddress("0x10").String(),
						types.StringToAddress("0x20").String()},
					amounts: []string{"10", "20"},
				}
			},
			err: rootHelper.ErrMandatoryStakeToken.Error(),
		},
		{
			// stake token address omitted
			buildParamsFn: func() *fundParams {
				return &fundParams{
					mintStakeToken: true,
					stakeTokenAddr: "0xA",
					addresses: []string{
						types.StringToAddress("0x10").String(),
						types.StringToAddress("0x20").String()},
					amounts: []string{"10", "20"},
				}
			},
			err: "invalid stake token address is provided",
		},
		{
			// valid scenario
			buildParamsFn: func() *fundParams {
				return &fundParams{
					addresses: []string{
						types.StringToAddress("0x10").String(),
						types.StringToAddress("0x20").String()},
					amounts: []string{"10", "20"},
				}
			},
			err: "",
		},
	}

	for i, c := range cases {
		c := c
		i := i

		t.Run(fmt.Sprintf("case#%d", i+1), func(t *testing.T) {
			t.Parallel()

			fp := c.buildParamsFn()

			err := fp.validateFlags()
			if c.err != "" {
				require.ErrorContains(t, err, c.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
