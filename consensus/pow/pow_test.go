package pow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/types"
)

func TestSeal(t *testing.T) {
	// t.Skip()

	c := &Pow{min: 1000000, max: 1500000}
	h := &types.Header{
		Number: 10,
	}

	// c.Prepare(nil, h)
	b := &types.Block{
		Header: h,
	}

	now := time.Now()
	c.Seal(b, context.Background())
	fmt.Println(time.Since(now))
}
