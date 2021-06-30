package system

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/state/runtime"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

type contractSetupParams struct {
	depth  int
	origin types.Address
	from   types.Address
	to     types.Address
	value  *big.Int
	gas    uint64
	code   []byte
}

func setupContract(params contractSetupParams) *runtime.Contract {
	return runtime.NewContract(
		params.depth,
		params.origin,
		params.from,
		params.to,
		params.value,
		params.gas,
		params.code,
	)
}

func TestSystem_CanRun(t *testing.T) {
	testTable := []struct {
		name     string
		contract *runtime.Contract
		canRun   bool
	}{
		{
			"Valid System runtime address (Staking)",
			setupContract(
				contractSetupParams{
					depth:  0,
					origin: types.StringToAddress("0"),
					from:   types.StringToAddress("0"),
					to:     types.StringToAddress(StakingAddress), // Staking handler
					value:  nil,
					gas:    0,
					code:   nil,
				},
			),
			true,
		},
		{
			"Valid System runtime address (Unstaking)",
			setupContract(
				contractSetupParams{
					depth:  0,
					origin: types.StringToAddress("0"),
					from:   types.StringToAddress("0"),
					to:     types.StringToAddress(UnstakingAddress), // Unstaking handler
					value:  nil,
					gas:    0,
					code:   nil,
				},
			),
			true,
		},
		{
			"Invalid System runtime address",
			setupContract(
				contractSetupParams{
					depth:  0,
					origin: types.StringToAddress("0"),
					from:   types.StringToAddress("0"),
					to:     types.StringToAddress("9999"), // Invalid handler
					value:  nil,
					gas:    0,
					code:   nil,
				},
			),
			false,
		},
	}

	systemRuntime := NewSystem()

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			runResult := systemRuntime.CanRun(testCase.contract, nil, nil)
			assert.Equalf(
				t,
				testCase.canRun,
				runResult,
				"Runtime doesn't recognize address %s",
				testCase.contract.CodeAddress,
			)
		})
	}
}
