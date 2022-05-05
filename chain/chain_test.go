package chain

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
)

var emptyAddr types.Address

func addr(str string) types.Address {
	return types.StringToAddress(str)
}

func hash(str string) types.Hash {
	return types.StringToHash(str)
}

func TestGenesisAlloc(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input  string
		output map[types.Address]GenesisAccount
	}{
		{
			input: `{
				"0x0000000000000000000000000000000000000000": {
					"balance": "0x11"
				}
			}`,
			output: map[types.Address]GenesisAccount{
				emptyAddr: {
					Balance: big.NewInt(17),
				},
			},
		},
		{
			input: `{
				"0x0000000000000000000000000000000000000000": {
					"balance": "0x11",
					"nonce": "0x100",
					"storage": {
						"` + hash("1").String() + `": "` + hash("3").String() + `",
						"` + hash("2").String() + `": "` + hash("4").String() + `"
					}
				}
			}`,
			output: map[types.Address]GenesisAccount{
				emptyAddr: {
					Balance: big.NewInt(17),
					Nonce:   256,
					Storage: map[types.Hash]types.Hash{
						hash("1"): hash("3"),
						hash("2"): hash("4"),
					},
				},
			},
		},
		{
			input: `{
				"0x0000000000000000000000000000000000000000": {
					"balance": "0x11"
				},
				"0x0000000000000000000000000000000000000001": {
					"balance": "0x12"
				}
			}`,
			output: map[types.Address]GenesisAccount{
				addr("0"): {
					Balance: big.NewInt(17),
				},
				addr("1"): {
					Balance: big.NewInt(18),
				},
			},
		},
	}

	for _, c := range cases {
		c := c

		t.Run("", func(t *testing.T) {
			t.Parallel()

			var dec map[types.Address]GenesisAccount
			if err := json.Unmarshal([]byte(c.input), &dec); err != nil {
				if c.output != nil {
					t.Fatal(err)
				}
			} else if !reflect.DeepEqual(dec, c.output) {
				t.Fatal("bad")
			}
		})
	}
}

func TestGenesisX(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input  string
		output *Genesis
	}{
		{
			input: `{
				"difficulty": "0x12",
				"gasLimit": "0x11",
				"alloc": {
					"0x0000000000000000000000000000000000000000": {
						"balance": "0x11"
					},
					"0x0000000000000000000000000000000000000001": {
						"balance": "0x12"
					}
				}
			}`,
			output: &Genesis{
				Difficulty: 18,
				GasLimit:   17,
				Alloc: map[types.Address]*GenesisAccount{
					emptyAddr: {
						Balance: big.NewInt(17),
					},
					addr("1"): {
						Balance: big.NewInt(18),
					},
				},
			},
		},
	}

	for _, c := range cases {
		c := c

		t.Run("", func(t *testing.T) {
			t.Parallel()

			var dec *Genesis
			if err := json.Unmarshal([]byte(c.input), &dec); err != nil {
				if c.output != nil {
					t.Fatal(err)
				}
			} else if !reflect.DeepEqual(dec, c.output) {
				t.Fatal("bad")
			}
		})
	}
}

func TestChainFolder(t *testing.T) {
	// it should be able to parse all the chains in the ./chains folder
	files, err := ioutil.ReadDir("./chains")
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		name := strings.TrimSuffix(f.Name(), ".json")
		if _, err := ImportFromName(name); err != nil {
			t.Fatalf("Failed to parse %s: %v", name, err)
		}
	}
}
