package staking

import (
	"testing"

	"github.com/0xPolygon/polygon-sdk/helper/hex"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
)

func TestGetStorageMappingIndex(t *testing.T) {

	expectedOutput := "0xc9c9d38ffc86e54587ebbb0f50fbfaeda01172f2ed3d3093531d3abcc205314b"

	address := types.StringToAddress("12345")
	slot := 0

	output := getAddressMapping(address, int64(slot))

	output2 := types.BytesToHash(output)

	hexValue := hex.EncodeToHex(output)

	assert.Equalf(t, expectedOutput, hexValue, "Not equal")
	assert.Equalf(t, expectedOutput, output2.String(), "Not equal 2")
}
