package calltracer

import (
	"errors"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

func TestCallTracer_Cancel(t *testing.T) {
	t.Parallel()

	err := errors.New("timeout")

	tracer := &CallTracer{}

	require.Nil(t, tracer.reason)
	require.False(t, tracer.stop)
	require.False(t, tracer.cancelled())

	tracer.Cancel(err)

	require.Equal(t, err, tracer.reason)
	require.True(t, tracer.stop)
	require.True(t, tracer.cancelled())
}

func TestCallTracer_Clear(t *testing.T) {
	t.Parallel()

	tracer := &CallTracer{}
	tracer.call = &Call{}

	tracer.Clear()

	require.Nil(t, tracer.call)
}

// CallStart function correctly initializes a new Call object with the provided parameters
func TestCallTracer_CallStart(t *testing.T) {
	t.Parallel()

	t.Run("call_start_initializes_call_object", func(t *testing.T) {
		t.Parallel()

		c := &CallTracer{}

		var (
			depth    = 1
			from     = types.StringToAddress("0xFrom")
			to       = types.StringToAddress("0xTo")
			callType = 0
			gas      = uint64(100000)
			value    = big.NewInt(100)
			input    = []byte("input")
		)

		c.CallStart(depth, from, to, callType, gas, value, input)

		expectedCall := &Call{
			Type:     "CALL",
			From:     from.String(),
			To:       to.String(),
			Value:    "0x64",
			Gas:      "0x186a0",
			GasUsed:  "",
			Input:    "0x696e707574",
			Output:   "",
			Calls:    nil,
			startGas: gas,
		}

		require.Equal(t, expectedCall, c.call)
	})

	t.Run("call_start_sets_parent_when_depth_greater_than_1", func(t *testing.T) {
		t.Parallel()

		c := &CallTracer{}

		var (
			depth    = 2
			from     = types.StringToAddress("0xFrom")
			to       = types.StringToAddress("0xTo")
			callType = 0
			gas      = uint64(100000)
			value    = big.NewInt(100)
			input    = []byte("input")
		)

		parentCall := &Call{
			Type:     "CALL",
			From:     from.String(),
			To:       to.String(),
			Value:    "0x64",
			Gas:      "0x186a0",
			GasUsed:  "",
			Input:    "0x696e707574",
			Output:   "",
			Calls:    nil,
			startGas: gas,
		}
		c.activeCall = parentCall

		c.CallStart(depth, from, to, callType, gas, value, input)

		expectedCall := &Call{
			Type:     "CALL",
			From:     from.String(),
			To:       to.String(),
			Value:    "0x64",
			Gas:      "0x186a0",
			GasUsed:  "",
			Input:    "0x696e707574",
			Output:   "",
			Calls:    nil,
			startGas: gas,
			parent:   parentCall,
		}

		require.Equal(t, expectedCall, c.activeCall)
		require.Equal(t, []*Call{expectedCall}, parentCall.Calls)
	})

	t.Run("call_start_handles_nil_value_parameter_for_value_and_input", func(t *testing.T) {
		t.Parallel()

		c := &CallTracer{}

		var (
			depth    = 1
			from     = types.Address{}
			to       = types.Address{}
			callType = 0
			gas      = uint64(100000)
		)

		c.CallStart(depth, from, to, callType, gas, nil, nil)

		expectedCall := &Call{
			Type:     "CALL",
			From:     from.String(),
			To:       to.String(),
			Value:    "0x0",
			Gas:      "0x186a0",
			GasUsed:  "",
			Input:    "0x",
			Output:   "",
			Calls:    nil,
			startGas: gas,
		}

		require.Equal(t, expectedCall, c.call)
	})

	t.Run("call_start_handles_unknown_call_type", func(t *testing.T) {
		t.Parallel()

		c := &CallTracer{}

		var (
			depth    = 1
			from     = types.Address{}
			to       = types.Address{}
			callType = 999 // unknown type
			gas      = uint64(100000)
			value    = big.NewInt(100)
			input    = []byte("input")
		)

		c.CallStart(depth, from, to, callType, gas, value, input)

		expectedCall := &Call{
			Type:     "UNKNOWN",
			From:     from.String(),
			To:       to.String(),
			Value:    "0x64",
			Gas:      "0x186a0",
			GasUsed:  "",
			Input:    "0x696e707574",
			Output:   "",
			Calls:    nil,
			startGas: gas,
		}

		require.Equal(t, expectedCall, c.call)
	})
}

func TestCallTracer_CallEnd(t *testing.T) {
	t.Parallel()

	output := []byte("output")
	err := errors.New("error")

	t.Run("call_end_when_depth_is_1_no_error_activeAvailableGas_higher_than_start_gas", func(t *testing.T) {
		t.Parallel()

		tracer := &CallTracer{}
		tracer.activeAvailableGas = 2000
		tracer.activeCall = &Call{
			startGas: 1000,
		}

		tracer.CallEnd(1, output, nil)

		require.Equal(t, uint64(0), tracer.activeGas)
		require.Equal(t, hex.EncodeToHex(output), tracer.activeCall.Output)
		require.Equal(t, "0x0", tracer.activeCall.GasUsed)
	})

	t.Run("call_end_when_depth_is_1_error_activeAvailableGas_higher_than_start_gas", func(t *testing.T) {
		t.Parallel()

		tracer := &CallTracer{}
		tracer.activeAvailableGas = 2000
		tracer.activeCall = &Call{
			startGas: 1000,
		}

		tracer.CallEnd(1, output, err)

		require.Equal(t, uint64(0), tracer.activeGas)
		require.Equal(t, hex.EncodeToHex(output), tracer.activeCall.Output)
		require.Equal(t, "0x0", tracer.activeCall.GasUsed)
		require.True(t, tracer.stop)
		require.Equal(t, err, tracer.reason)
	})

	t.Run("call_end_when_depth_is_1_no_error_activeAvailableGas_lower_than_start_gas", func(t *testing.T) {
		t.Parallel()

		tracer := &CallTracer{}
		tracer.activeAvailableGas = 1000
		tracer.activeCall = &Call{
			startGas: 2000,
		}

		tracer.CallEnd(1, output, nil)

		require.Equal(t, uint64(0), tracer.activeGas)
		require.Equal(t, hex.EncodeToHex(output), tracer.activeCall.Output)
		require.Equal(t, hex.EncodeUint64(1000), tracer.activeCall.GasUsed)
	})

	t.Run("call_end_when_depth_is_1_error_activeAvailableGas_lower_than_start_gas", func(t *testing.T) {
		t.Parallel()

		tracer := &CallTracer{}
		tracer.activeAvailableGas = 1000
		tracer.activeCall = &Call{
			startGas: 2000,
		}

		tracer.CallEnd(1, output, err)

		require.Equal(t, uint64(0), tracer.activeGas)
		require.Equal(t, hex.EncodeToHex(output), tracer.activeCall.Output)
		require.Equal(t, hex.EncodeUint64(1000), tracer.activeCall.GasUsed)
		require.True(t, tracer.stop)
		require.Equal(t, err, tracer.reason)
	})

	t.Run("call_end_when_depth_is_2_no_error", func(t *testing.T) {
		t.Parallel()

		tracer := &CallTracer{}
		tracer.activeAvailableGas = 2000
		tracer.activeCall = &Call{
			startGas: 1000,
			parent: &Call{
				startGas: 500,
				Output:   hex.EncodeToHex(output),
				GasUsed:  "0x0",
			},
		}

		tracer.CallEnd(2, output, nil)

		require.Equal(t, uint64(0), tracer.activeGas)
		require.Equal(t, hex.EncodeToHex(output), tracer.activeCall.Output)
		require.Equal(t, "0x0", tracer.activeCall.GasUsed)
		require.Equal(t, uint64(500), tracer.activeCall.startGas)
	})
}
