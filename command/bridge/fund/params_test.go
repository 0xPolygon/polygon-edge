package fund

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

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
					rawAddresses: []string{"0x1", "0x2"},
					amounts:      []string{"10"},
				}
			},
			err: errInconsistentLength.Error(),
		},
		{
			// address contains invalid characters
			buildParamsFn: func() *fundParams {
				return &fundParams{
					rawAddresses: []string{"0x10", "0x20"},
					amounts:      []string{"10", "20"},
				}
			},
			err: "address \x10 has invalid length",
		},
		{
			// valid scenario
			buildParamsFn: func() *fundParams {
				return &fundParams{
					rawAddresses: []string{
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
