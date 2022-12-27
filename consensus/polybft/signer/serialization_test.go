package bls

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_G1FromBigIntInfinity(t *testing.T) {
	t.Parallel()

	g1, err := G1FromBigInt([2]*big.Int{big.NewInt(0), big.NewInt(0)})
	require.NoError(t, err)

	zero := newFp(0, 0, 0, 0)

	assert.True(t, g1.Y.IsEqual(&r1))
	assert.True(t, g1.X.IsEqual(&zero))
	assert.True(t, g1.Z.IsEqual(&zero))
}

func Test_G2FromBigIntInfinity(t *testing.T) {
	t.Parallel()

	g2, err := G2FromBigInt([4]*big.Int{big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0)})
	require.NoError(t, err)

	zero := newFp(0, 0, 0, 0)

	assert.True(t, g2.Y.D[0].IsEqual(&r1))
	assert.True(t, g2.X.D[0].IsEqual(&zero))
	assert.True(t, g2.X.D[1].IsEqual(&zero))
	assert.True(t, g2.Y.D[1].IsEqual(&zero))
	assert.True(t, g2.Z.D[0].IsEqual(&zero))
	assert.True(t, g2.Z.D[1].IsEqual(&zero))
}

func Test_BytesToBigInt2WrongSize(t *testing.T) {
	_, err := BytesToBigInt2(nil)
	require.Error(t, err)

	_, err = BytesToBigInt2(make([]byte, 63))
	require.Error(t, err)

	_, err = BytesToBigInt2(make([]byte, 65))
	require.Error(t, err)

	_, err = BytesToBigInt2(make([]byte, 64))
	require.NoError(t, err)
}

func Test_BytesToBigInt4WrongSize(t *testing.T) {
	_, err := BytesToBigInt4(nil)
	require.Error(t, err)

	_, err = BytesToBigInt4(make([]byte, 127))
	require.Error(t, err)

	_, err = BytesToBigInt4(make([]byte, 129))
	require.Error(t, err)

	_, err = BytesToBigInt4(make([]byte, 128))
	require.NoError(t, err)
}
