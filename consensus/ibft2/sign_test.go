package ibft2

import (
	"testing"

	"github.com/0xPolygon/minimal/types"
)

func TestHeaderSealSign(t *testing.T) {
	pool := newTesterAccountPool(2)

	h := &types.Header{}
	putIbftExtraValidators(h, pool.ValidatorSet())

	sealedHeader, err := writeSeal(pool.indx(0).priv, h)
	if err != nil {
		t.Fatal(err)
	}

	addr, err := ecrecover(sealedHeader)
	if err != nil {
		t.Fatal(err)
	}

	if pool.indx(0).Address() != addr {
		t.Fatal("bad")
	}
}
