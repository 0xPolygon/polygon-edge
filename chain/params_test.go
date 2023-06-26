package chain

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/types"
)

func TestParamsForks(t *testing.T) {
	cases := []struct {
		input  string
		output *Forks
	}{
		{
			input: `{
				"homestead": { "block": 1000 }
			}`,
			output: &Forks{
				Homestead: NewFork(1000),
			},
		},
	}

	for _, c := range cases {
		var dec *Forks
		if err := json.Unmarshal([]byte(c.input), &dec); err != nil {
			if c.output != nil {
				t.Fatal(err)
			}
		} else if !reflect.DeepEqual(dec, c.output) {
			t.Fatal("bad")
		}
	}
}

func TestParamsForksInTime(t *testing.T) {
	f := Forks{
		Homestead:      NewFork(0),
		Byzantium:      NewFork(1000),
		Constantinople: NewFork(1001),
		EIP150:         NewFork(2000),
	}

	ff := f.At(1000)

	expect := func(name string, found bool, expect bool) {
		if expect != found {
			t.Fatalf("fork %s should be %v but found %v", name, expect, found)
		}
	}

	expect("homestead", ff.Homestead, true)
	expect("byzantium", ff.Byzantium, true)
	expect("constantinople", ff.Constantinople, false)
	expect("eip150", ff.EIP150, false)
}

func TestParams_CalculateBurnContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		burnContract map[uint64]types.Address
		block        uint64
		want         types.Address
		wantErr      bool
	}{
		{
			name:         "no addresses in the list",
			burnContract: map[uint64]types.Address{},
			block:        10,
			want:         types.ZeroAddress,
			wantErr:      true,
		},
		{
			name: "last address is used",
			burnContract: map[uint64]types.Address{
				15: types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c9db1"),
			},
			block:   10,
			want:    types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c9db1"),
			wantErr: false,
		},
		{
			name: "first address is used",
			burnContract: map[uint64]types.Address{
				5:  types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c9db2"),
				15: types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c9db1"),
			},
			block:   10,
			want:    types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c9db2"),
			wantErr: false,
		},
		{
			name: "same block as key",
			burnContract: map[uint64]types.Address{
				5:  types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c4db2"),
				10: types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c5db1"),
				15: types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c6db0"),
			},
			block:   10,
			want:    types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c5db1"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &Params{
				BurnContract: tt.burnContract,
			}

			got, err := p.CalculateBurnContract(tt.block)
			if tt.wantErr {
				require.Error(t, err, "CalculateBurnContract() error = %v, wantErr %v", err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got, "CalculateBurnContract() got = %v, want %v", got, tt.want)
			}
		})
	}
}
