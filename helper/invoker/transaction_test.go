package invoker

import (
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestRecover(t *testing.T) {

	// r: 0x8b1b4920a872ff8fdd66a35ef370a9d9113af4234b91ab0087c51e8362287073
	// s: 0x68da649ea8d0444671dc2e3ed9b40ab9b0eee08d745a5527bdb92f0dff746d9c
	// v: false
	// commit: 0x6ca2b29e76ec69ab132c23acdafc5650de9f0ee2aa6ada70031962b37e24b026
	// user: 0x3D09c91F44C87C30901dDB742D99f168F5AEEf01
	// invoker: 0xC66298c7a6aDE36b928d6e9598Af7804611AbDC0

	r := new(big.Int).SetBytes(hex.MustDecodeHex("0x8b1b4920a872ff8fdd66a35ef370a9d9113af4234b91ab0087c51e8362287073"))
	s := new(big.Int).SetBytes(hex.MustDecodeHex("0x68da649ea8d0444671dc2e3ed9b40ab9b0eee08d745a5527bdb92f0dff746d9c"))
	commit := hex.MustDecodeHex("0x6ca2b29e76ec69ab132c23acdafc5650de9f0ee2aa6ada70031962b37e24b026")

	invokerAddr := types.StringToAddress("0xC66298c7a6aDE36b928d6e9598Af7804611AbDC0")

	is := InvokerSignature{
		R: r,
		S: s,
		V: false,
	}

	signAddr := is.Recover(commit, invokerAddr)
	require.Equal(t, "0x3D09c91F44C87C30901dDB742D99f168F5AEEf01", signAddr.String())
}
