package precompiled

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/umbracle/minimal/chain"
)

func TestPrecompiled(t *testing.T) {
	var cases = []struct {
		Builtin     string
		Precompiled *Precompiled
		Error       bool
	}{
		{
			Builtin: `{
				"name": "sha256",
				"pricing": {
					"base": 0,
					"word": 0
				}	
			}`,
			Precompiled: &Precompiled{
				Backend: &sha256hash{
					Base: 0,
					Word: 0,
				},
			},
			Error: false,
		},
		{
			Builtin: `{
				"name": "sha256",
				"pricing": {
					"base": 10,
					"word": 11
				}	
			}`,
			Precompiled: &Precompiled{
				Backend: &sha256hash{
					Base: 10,
					Word: 11,
				},
			},
			Error: false,
		},
		{
			Builtin: `{
				"name": "modexp",
				"pricing": {
					"divisor": 10
				}
			}`,
			Precompiled: &Precompiled{
				Backend: &modExp{
					Divisor: 10,
				},
			},
			Error: false,
		},
		{
			Builtin: `{
				"name": "alt_bn128_pairing",
				"pricing": {
					"base": 1,
					"pair": 2
				}
			}`,
			Precompiled: &Precompiled{
				Backend: &bn256Pairing{
					Base: 1,
					Pair: 2,
				},
			},
			Error: false,
		},
		{
			Builtin: `{
				"name": "unknown"
			}`,
			Error: true,
		},
		{
			Builtin: `{
				"name": "alt_bn128_pairing",
				"pricing": {
					"unkwown": 1
				}
			}`,
			Error: true,
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			var builtin *chain.Builtin
			if err := json.Unmarshal([]byte(c.Builtin), &builtin); err != nil {
				t.Fatal(err)
			}
			p, err := CreatePrecompiled(builtin)
			if err != nil && !c.Error {
				t.Fatalf("error not expected: %v", err)
			} else if err == nil && c.Error {
				t.Fatal("expected error")
			}

			if !c.Error {
				if !reflect.DeepEqual(p, c.Precompiled) {
					t.Fatal("bad")
				}
			}
		})
	}
}
