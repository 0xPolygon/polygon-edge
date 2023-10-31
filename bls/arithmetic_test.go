package bls

import (
	"bytes"
	"embed"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/helper/common"
)

//go:embed testcases/*
var testcases embed.FS

func TestHashToPoint(t *testing.T) {
	var hashToPointCases []struct {
		Msg    string
		Domain string
		X      string
		Y      string
	}

	data, err := testcases.ReadFile("testcases/hashToPoint.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &hashToPointCases))

	for _, c := range hashToPointCases {
		msg, _ := hex.DecodeString(c.Msg[2:])
		domain, _ := hex.DecodeString(c.Domain[2:])

		x, _ := new(big.Int).SetString(c.X, 10)
		y, _ := new(big.Int).SetString(c.Y, 10)

		g1, err := hashToPoint(msg, domain)
		require.NoError(t, err)

		buf := g1.Marshal()

		xBuf := common.PadLeftOrTrim(x.Bytes(), 32)
		if !bytes.Equal(buf[:32], xBuf) {
			t.Fatal("point x not correct")
		}

		yBuf := common.PadLeftOrTrim(y.Bytes(), 32)
		if !bytes.Equal(buf[32:], yBuf) {
			t.Fatal("point y is not correct")
		}
	}
}
