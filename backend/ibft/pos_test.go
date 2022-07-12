package ibft

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	TestEpochSize = 10
)

func TestGetEpoch(t *testing.T) {
	tests := []struct {
		num   uint64
		epoch uint64
	}{
		// genesis
		{
			num:   0,
			epoch: 0,
		},
		// first number
		{
			num:   1,
			epoch: 1,
		},
		{
			num:   5,
			epoch: 1,
		},
		// end of first epoch
		{
			num:   10,
			epoch: 1,
		},
		// first of second epoch
		{
			num:   11,
			epoch: 2,
		},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("GetEpoch should return %d for number %d", tt.epoch, tt.num)
		t.Run(name, func(t *testing.T) {
			ibft := &Ibft{
				epochSize: TestEpochSize,
			}
			res := ibft.GetEpoch(tt.num)
			assert.Equal(t, tt.epoch, res)
		})
	}
}

func TestIsFirstOfEpoch(t *testing.T) {
	tests := []struct {
		num     uint64
		isFirst bool
	}{
		// genesis
		{
			num:     0,
			isFirst: false,
		},
		// first number
		{
			num:     1,
			isFirst: true,
		},
		{
			num:     5,
			isFirst: false,
		},
		// end of first epoch
		{
			num:     10,
			isFirst: false,
		},
		// first of second epoch
		{
			num:     11,
			isFirst: true,
		},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("IsFirstOfEpoch should return %t for number %d", tt.isFirst, tt.num)
		t.Run(name, func(t *testing.T) {
			ibft := &Ibft{
				epochSize: TestEpochSize,
			}
			assert.Equal(t, tt.isFirst, tt.num%ibft.epochSize == 1)
		})
	}
}

func TestIsLastOfEpoch(t *testing.T) {
	tests := []struct {
		num    uint64
		isLast bool
	}{
		// genesis
		{
			num:    0,
			isLast: false,
		},
		// first number
		{
			num:    1,
			isLast: false,
		},
		{
			num:    5,
			isLast: false,
		},
		// end of first epoch
		{
			num:    10,
			isLast: true,
		},
		// first of second epoch
		{
			num:    11,
			isLast: false,
		},
		// last of second epoch
		{
			num:    20,
			isLast: true,
		},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("IsLastOfEpoch should return %t for number %d", tt.isLast, tt.num)
		t.Run(name, func(t *testing.T) {
			ibft := &Ibft{
				epochSize: TestEpochSize,
			}
			res := ibft.IsLastOfEpoch(tt.num)
			assert.Equal(t, tt.isLast, res)
		})
	}
}
