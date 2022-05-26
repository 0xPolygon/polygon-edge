package staking

import (
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
	chaosFactor := int64(1000000000000000)
	resultSet, err := u.GenHash(chaosFactor)
	assert.NoError(t, err)
	assert.Equal(t, len(resultSet), 21)

	fmt.Printf("resultSet: +%v \n", resultSet)
}
