package evm

import (
	"strings"
	"testing"
)

func TestIsPush(t *testing.T) {
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
