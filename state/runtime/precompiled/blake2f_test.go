package precompiled

import (
	"bytes"
	"testing"

	"go.uber.org/goleak"
)

func TestBlake2f(t *testing.T) {
	defer goleak.VerifyNone(t)

	b := &blake2f{}

	ReadTestCase(t, "blake2f.json", func(t *testing.T, c *TestCase) {
		t.Helper()

		out, err := b.run(c.Input)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(c.Expected, out) {
			t.Fatal("bad")
		}
	})
}
