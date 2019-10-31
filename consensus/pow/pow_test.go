package pow

import (
	"context"
	"testing"

	"github.com/umbracle/minimal/types"
)

func TestSeal(t *testing.T) {
	// t.Skip()

	c, _ := Factory(context.Background(), nil)
	h := &types.Header{
		Number: 10,
	}

	// c.Prepare(nil, h)
	b := &types.Block{
		Header: h,
	}
	c.Seal(context.Background(), b)
}
