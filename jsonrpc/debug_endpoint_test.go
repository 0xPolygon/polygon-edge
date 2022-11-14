package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDebugTraceConfigDecode(t *testing.T) {
	tests := []struct {
		input    string
		expected TraceConfig
	}{
		{
			// default
			input: `{}`,
			expected: TraceConfig{
				EnableMemory:     false,
				DisableStack:     false,
				DisableStorage:   false,
				EnableReturnData: false,
			},
		},
		{
			input: `{
				"enableMemory": true
			}`,
			expected: TraceConfig{
				EnableMemory:     true,
				DisableStack:     false,
				DisableStorage:   false,
				EnableReturnData: false,
			},
		},
		{
			input: `{
				"disableStack": true
			}`,
			expected: TraceConfig{
				EnableMemory:     false,
				DisableStack:     true,
				DisableStorage:   false,
				EnableReturnData: false,
			},
		},
		{
			input: `{
				"disableStorage": true
			}`,
			expected: TraceConfig{
				EnableMemory:     false,
				DisableStack:     false,
				DisableStorage:   true,
				EnableReturnData: false,
			},
		},
		{
			input: `{
				"enableReturnData": true
			}`,
			expected: TraceConfig{
				EnableMemory:     false,
				DisableStack:     false,
				DisableStorage:   false,
				EnableReturnData: true,
			},
		},
		{
			input: `{
				"enableMemory": true,
				"disableStack": true,
				"disableStorage": true,
				"enableReturnData": true
			}`,
			expected: TraceConfig{
				EnableMemory:     true,
				DisableStack:     true,
				DisableStorage:   true,
				EnableReturnData: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := TraceConfig{}

			assert.NoError(
				t,
				json.Unmarshal(
					[]byte(test.input),
					&result,
				),
			)

			assert.Equal(
				t,
				test.expected,
				result,
			)
		})
	}
}
