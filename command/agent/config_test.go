package agent

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestConfigMerge(t *testing.T) {
	cases := []struct {
		src *Config
		dst *Config
		res *Config
	}{
		{
			&Config{
				Chain: "a",
			},
			&Config{
				Chain: "b",
			},
			&Config{
				Chain: "b",
			},
		},
		{
			&Config{},
			&Config{
				Discovery: map[string]BackendConfig{
					"a": BackendConfig{
						"b": 1,
					},
				},
			},
			&Config{
				Discovery: map[string]BackendConfig{
					"a": BackendConfig{
						"b": 1,
					},
				},
			},
		},
		{
			&Config{
				Discovery: map[string]BackendConfig{
					"a": BackendConfig{
						"b": 1,
					},
				},
			},
			&Config{
				Discovery: map[string]BackendConfig{
					"a": BackendConfig{
						"b": 2,
					},
				},
			},
			&Config{
				Discovery: map[string]BackendConfig{
					"a": BackendConfig{
						"b": 2,
					},
				},
			},
		},
		{
			&Config{
				Protocols: map[string]BackendConfig{
					"a": BackendConfig{
						"a": 1,
					},
				},
			},
			&Config{
				Protocols: map[string]BackendConfig{
					"a": BackendConfig{
						"b": 2,
					},
				},
			},
			&Config{
				Protocols: map[string]BackendConfig{
					"a": BackendConfig{
						"a": 1,
						"b": 2,
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			if err := c.src.merge(c.dst); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c.src, c.res) {
				t.Fatal("bad")
			}
		})
	}
}

func TestConfigFromFile(t *testing.T) {
	cases := []struct {
		data   string
		config *Config
	}{
		{
			`{
				"data_dir": "xx"
			}`,
			&Config{
				DataDir: "xx",
			},
		},
		{
			`{
				"telemetry": {
					"prometheus_port": 1200
				}	
			}`,
			&Config{
				Telemetry: &Telemetry{
					PrometheusPort: 1200,
				},
			},
		},
		{
			`{
				"protocols": {
					"a": {
						"b": 1,
						"c": 2
					},
					"d": {
						"e": 3,
						"f": 4
					}
				}
			}`,
			&Config{
				Protocols: map[string]BackendConfig{
					"a": BackendConfig{
						"b": float64(1),
						"c": float64(2),
					},
					"d": BackendConfig{
						"e": float64(3),
						"f": float64(4),
					},
				},
			},
		},
		{
			`{
				"blockchain": {
					"backend": "xx",
					"config": {
						"a": 1,
						"b": 2
					}
				}
			}`,
			&Config{
				Blockchain: &BlockchainConfig{
					Backend: "xx",
					Config: BackendConfig{
						"a": float64(1),
						"b": float64(2),
					},
				},
			},
		},
		{
			`{
				"consensus": {
					"a": 1,
					"b": "2"
				}
			}`,
			&Config{
				Consensus: BackendConfig{
					"a": float64(1),
					"b": "2",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			var config *Config
			if err := json.Unmarshal([]byte(c.data), &config); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c.config, config) {
				t.Fatal("bad")
			}
		})
	}
}
