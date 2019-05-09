package jsonrpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetServerConfig(t *testing.T) {

	cases := []struct {
		config    map[string]interface{}
		result    map[string]interface{}
		err       bool
		disabled  bool
		endpoints []string
	}{
		{
			config: map[string]interface{}{
				"disabled": true,
			},
			disabled: true,
		},
		{
			config:    nil,
			result:    nil,
			disabled:  false,
			endpoints: defaultEndpoints,
		},
		{
			config: map[string]interface{}{
				"path": "/tmp/data",
			},
			result: map[string]interface{}{
				"path": "/tmp/data",
			},
			endpoints: defaultEndpoints,
		},
		{
			config: map[string]interface{}{
				"endpoints": []string{"a", "b", "c"},
				"path":      "/tmp/data",
			},
			result: map[string]interface{}{
				"path": "/tmp/data",
			},
			endpoints: []string{"a", "b", "c"},
		},
	}

	field := "xx"

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			config := map[string]interface{}{
				field: c.config,
			}

			result, endpoints, disabled, err := getServerConfig(config, field)
			if err != nil && !c.err {
				t.Fatal("error not expected")
			}
			if err == nil && c.err {
				t.Fatal("error expected")
			}

			assert.NoError(t, err)
			assert.Equal(t, disabled, c.disabled)
			assert.Equal(t, result, c.result)
			assert.Equal(t, endpoints, c.endpoints)
		})
	}
}
