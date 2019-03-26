package pow

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestPow(t *testing.T) {
	c := &Pow{
		min: 80,
		max: 120,
	}

	parent := &types.Header{
		Number:     big.NewInt(0),
		Time:       big.NewInt(0),
		Difficulty: big.NewInt(0),
	}

	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     big.NewInt(1),
		Time:       big.NewInt(1),
	}

	if err := c.Prepare(parent, header); err != nil {
		t.Fatal(err)
	}
	b, err := c.Seal(context.Background(), types.NewBlockWithHeader(header))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.VerifyHeader(parent, b.Header(), true, true); err != nil {
		t.Fatal(err)
	}
}
