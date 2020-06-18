package ibft

import (
	"math/big"
	"testing"
)

func TestViewCompare(t *testing.T) {
	// test equality
	srvView := &View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(1),
	}
	tarView := &View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(1),
	}
	if r := srvView.Cmp(tarView); r != 0 {
		t.Errorf("source(%v) should be equal to target(%v): have %v, want %v", srvView, tarView, r, 0)
	}

	// test larger Sequence
	tarView = &View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(1),
	}
	if r := srvView.Cmp(tarView); r != 1 {
		t.Errorf("source(%v) should be larger than target(%v): have %v, want %v", srvView, tarView, r, 1)
	}

	// test larger Round
	tarView = &View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(0),
	}
	if r := srvView.Cmp(tarView); r != 1 {
		t.Errorf("source(%v) should be larger than target(%v): have %v, want %v", srvView, tarView, r, 1)
	}

	// test smaller Sequence
	tarView = &View{
		Sequence: big.NewInt(3),
		Round:    big.NewInt(1),
	}
	if r := srvView.Cmp(tarView); r != -1 {
		t.Errorf("source(%v) should be smaller than target(%v): have %v, want %v", srvView, tarView, r, -1)
	}
	tarView = &View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(2),
	}
	if r := srvView.Cmp(tarView); r != -1 {
		t.Errorf("source(%v) should be smaller than target(%v): have %v, want %v", srvView, tarView, r, -1)
	}
}
