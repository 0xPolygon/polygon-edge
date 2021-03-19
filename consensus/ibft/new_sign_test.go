package ibft

import (
	"testing"

	"github.com/0xPolygon/minimal/types"
)

func TestHeaderSealSign(t *testing.T) {
	pool := &validatorsPool{}
	pool.add("A")

	h := &types.Header{}
	putIbftExtraValidators(h, pool.list())

	sealedHeader, err := writeSeal(pool.accounts["A"], h)
	if err != nil {
		t.Fatal(err)
	}

	addr, err := ecrecover(sealedHeader)
	if err != nil {
		t.Fatal(err)
	}

	if pool.address("A") != addr {
		t.Fatal("bad")
	}
}
