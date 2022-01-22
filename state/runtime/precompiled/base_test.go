//nolint: lll
package precompiled

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/stretchr/testify/assert"
)

type precompiledTest struct {
	Name     string
	Input    string
	Expected string
}

func testPrecompiled(t *testing.T, p contract, cases []precompiledTest) {
	t.Helper()

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			h, _ := hex.DecodeString(c.Input)
			found, err := p.run(h)

			assert.NoError(t, err)
			assert.Equal(t, c.Expected, hex.EncodeToString(found))
		})
	}
}

func TestECRecover(t *testing.T) {
	var tests = []precompiledTest{
		{
			Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
			Expected: "000000000000000000000000ceaccac640adf55b2028469bd36ba501f28b699d",
			Name:     "",
		},
	}

	p := &Precompiled{}
	testPrecompiled(t, &ecrecover{p}, tests)
}

func TestSha256(t *testing.T) {
	var tests = []precompiledTest{
		{
			Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
			Expected: "811c7003375852fabd0d362e40e68607a12bdabae61a7d068fe5fdd1dbbf2a5d",
			Name:     "128",
		},
	}

	testPrecompiled(t, &sha256h{}, tests)
}

func TestRipeMD(t *testing.T) {
	var tests = []precompiledTest{
		{
			Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
			Expected: "0000000000000000000000009215b8d9882ff46f0dfde6684d78e831467f65e6",
			Name:     "128",
		},
	}

	testPrecompiled(t, &ripemd160h{}, tests)
}

func TestIdentity(t *testing.T) {
	var tests = []precompiledTest{
		{
			Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
			Expected: "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
			Name:     "128",
		},
	}

	testPrecompiled(t, &identity{}, tests)
}
