package precompiled

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

func Test_BlsAggSignsVerification(t *testing.T) {
	b := &blsAggSignsVerification{}

	ReadTestCase(t, "blsAggSigns.json", func(t *testing.T, c *TestCase) {
		t.Helper()
		out, err := b.run(c.Input, types.ZeroAddress, nil)
		require.NoError(t, err)
		require.Equal(t, c.Expected, out)
	})
}
