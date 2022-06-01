package staking

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFactorial(t *testing.T) {
	fab := factorial(21)
	fmt.Println("fab21: ", fab)
}

func TestGenHash(t *testing.T) {
	u := NewUpHash(21)

	// fmt.Println(" head hash ", headHash)
	// factor := int64(binary.BigEndian.Uint64(headHash.Bytes()))

	// 0x15c9ceb72381fd0c62e5fc44fb82525a843509adf9a4dae0f3871240b61282fd

	// hash := types.BytesToHash([]byte("15c9ceb72381fd0c62e5fc44fb82525a843509adf9a4dae0f3871240b61282fd"))

	chaosFactor := int64(binary.BigEndian.Uint64([]byte("15c9ceb72381fd0c62e5fc44fb82525a843509adf9a4dae0f3871240b61282fd")))
	// chaosFactor := int64(1000000000000000)
	resultSet, err := u.GenHash(chaosFactor)
	assert.NoError(t, err)
	assert.Equal(t, len(resultSet), 21)

	fmt.Printf("resultSet: +%v \n", resultSet)
}
