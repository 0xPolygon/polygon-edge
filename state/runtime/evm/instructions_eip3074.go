package evm

import (
	"errors"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/invoker"
	"github.com/hashicorp/go-hclog"
)

var logger hclog.Logger

func init() {
	logger = hclog.New(&hclog.LoggerOptions{
		Name:   "evm",
		Output: hclog.DefaultOutput,
		Level:  hclog.Trace,
	})
}

func opAuth(op OpCode) instruction {
	return func(c *state) {

		logger.Info("opAuth", "op", op.String())

		c.authorized = nil

		commit := c.pop()
		v := c.pop()
		r := c.pop()
		s := c.pop()

		if !crypto.ValidateSignatureValues(byte(v.Uint64()), r, s) {
			c.exit(errors.New("invalid signature values"))
			return
		}

		vB := v.Int64() == 1

		is := invoker.InvokerSignature{
			R: r,
			S: s,
			V: vB,
		}

		signAddr := is.Recover(commit.Bytes(), c.msg.CodeAddress)

		logger.Info("signature values are valid",
			"commit", hex.EncodeToString(commit.Bytes()),
			"v", vB,
			"r", hex.EncodeToHex(r.Bytes()),
			"s", hex.EncodeToHex(s.Bytes()),
			"signAddr",
			signAddr.String())

		c.authorized = &signAddr

		c.ret = make([]byte, 12)
		c.ret = append(c.ret, signAddr.Bytes()...)

		c.push1().SetBytes(signAddr.Bytes())

		return
	}
}
