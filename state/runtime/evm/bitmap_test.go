package evm

import (
	"strings"
	"testing"

	"go.uber.org/goleak"
)

func TestIsPush(t *testing.T) {
	defer goleak.VerifyNone(t)

	num := 0

	for i := 0x0; i < 0xFF; i++ {
		if i>>5 == 3 {
			if !strings.HasPrefix(OpCode(i).String(), "PUSH") {
				t.Fatal("err")
			}
			num++
		}
	}

	if num != 32 {
		t.Fatal("bad")
	}
}
