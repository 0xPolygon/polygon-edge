package state

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/0xPolygon/polygon-edge/types"
)

func Test_checkDynamicFees(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseFee *big.Int
		tx      *types.Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "happy path",
			baseFee: big.NewInt(100),
			tx: &types.Transaction{
				GasFeeCap: big.NewInt(100),
				GasTipCap: big.NewInt(100),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				assert.NoError(t, err, i)
				return false
			},
		},
		{
			name:    "happy path with empty values",
			baseFee: big.NewInt(0),
			tx: &types.Transaction{
				GasFeeCap: big.NewInt(0),
				GasTipCap: big.NewInt(0),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				assert.NoError(t, err, i)
				return false
			},
		},
		{
			name:    "gas fee cap less than base fee",
			baseFee: big.NewInt(20),
			tx: &types.Transaction{
				GasFeeCap: big.NewInt(10),
				GasTipCap: big.NewInt(0),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				expectedError := fmt.Sprintf("max fee per gas less than block base fee: "+
					"address %s, GasFeeCap: 10, BaseFee: 20", types.ZeroAddress)
				assert.EqualError(t, err, expectedError, i)
				return true
			},
		},
		{
			name:    "gas fee cap less than tip cap",
			baseFee: big.NewInt(5),
			tx: &types.Transaction{
				GasFeeCap: big.NewInt(10),
				GasTipCap: big.NewInt(15),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				expectedError := fmt.Sprintf("max priority fee per gas higher than max fee per gas: "+
					"address %s, GasTipCap: 15, GasFeeCap: 10", types.ZeroAddress)
				assert.EqualError(t, err, expectedError, i)
				return true
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := checkDynamicFees(tt.tx, tt.baseFee)
			tt.wantErr(t, err, fmt.Sprintf("checkDynamicFees(%v, %v)", tt.tx, tt.baseFee))
		})
	}
}
