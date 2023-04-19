package precompiled

import (
	"bytes"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
)

func TestBlake2f(t *testing.T) {
	b := &blake2f{}

	ReadTestCase(t, "blake2f.json", func(t *testing.T, c *TestCase) {
		t.Helper()

		out, err := b.run(c.Input, types.ZeroAddress, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(c.Expected, out) {
			t.Fatal("bad")
		}
	})
}
