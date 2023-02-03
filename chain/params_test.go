package chain

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
)

func TestParamsForks(t *testing.T) {
	cases := []struct {
		input  string
		output *Forks
	}{
		{
			input: `{
				"homestead": 1000
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

func TestParams_CalculateBurntContract(t *testing.T) {
	tests := []struct {
		name          string
		burntContract map[string]string
		block         uint64
		want          types.Address
		wantErr       bool
	}{
		{
			name:          "no addresses in the list",
			burntContract: map[string]string{},
			block:         10,
			want:          types.ZeroAddress,
			wantErr:       true,
		},
		{
			name: "last address is used",
			burntContract: map[string]string{
				"15": "0x8888f1f195afa192cfee860698584c030f4c9db1",
			},
			block:   10,
			want:    types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c9db1"),
			wantErr: false,
		},
		{
			name: "first address is used",
			burntContract: map[string]string{
				"5":  "0x8888f1f195afa192cfee860698584c030f4c9db2",
				"15": "0x8888f1f195afa192cfee860698584c030f4c9db1",
			},
			block:   10,
			want:    types.StringToAddress("0x8888f1f195afa192cfee860698584c030f4c9db2"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Params{
				BurntContract: tt.burntContract,
			}

			got, err := p.CalculateBurntContract(tt.block)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateBurntContract() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CalculateBurntContract() got = %v, want %v", got, tt.want)
			}
		})
	}
}
