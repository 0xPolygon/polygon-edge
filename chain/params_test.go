package chain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestValidateChainID(t *testing.T) {
	for id, name := range chains {
		c, err := ImportFromName(strings.ToLower(name))
		if err == nil {
			if c.Params.ChainID != int(id) {
				t.Fatalf("Network id does not match for %s. Expected %d but found %d", name, c.Params.ChainID, id)
			}
		}
	}
}

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
