package blockbuilder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateGasLimit(t *testing.T) {
	tests := []struct {
		name             string
		blockGasTarget   uint64
		parentGasLimit   uint64
		expectedGasLimit uint64
	}{
		{
			name:             "should increase next gas limit towards target",
			blockGasTarget:   25000000,
			parentGasLimit:   20000000,
			expectedGasLimit: 20000000/1024 + 20000000,
		},
		{
			name:             "should decrease next gas limit towards target",
			blockGasTarget:   25000000,
			parentGasLimit:   26000000,
			expectedGasLimit: 26000000 - 26000000/1024,
		},
		{
			name:             "should not alter gas limit when exactly the same",
			blockGasTarget:   25000000,
			parentGasLimit:   25000000,
			expectedGasLimit: 25000000,
		},
		{
			name:             "should increase to the exact gas target if adding the delta surpasses it",
			blockGasTarget:   25000000 + 25000000/1024 - 100, // - 100 so that it takes less than the delta to reach it
			parentGasLimit:   25000000,
			expectedGasLimit: 25000000 + 25000000/1024 - 100,
		},
		{
			name:             "should decrease to the exact gas target if subtracting the delta surpasses it",
			blockGasTarget:   25000000 - 25000000/1024 + 100, // + 100 so that it takes less than the delta to reach it
			parentGasLimit:   25000000,
			expectedGasLimit: 25000000 - 25000000/1024 + 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := calculateGasLimit(tt.parentGasLimit, tt.blockGasTarget)
			assert.Equal(t, tt.expectedGasLimit, found)
		})
	}
}
