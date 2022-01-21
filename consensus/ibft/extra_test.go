package ibft

import (
	"reflect"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
)

func TestExtraEncoding(t *testing.T) {
	seal1 := types.StringToHash("1").Bytes()

	cases := []struct {
		extra []byte
		data  *IstanbulExtra
	}{
		{
			data: &IstanbulExtra{
				Validators: []types.Address{
					types.StringToAddress("1"),
				},
				Seal: seal1,
				CommittedSeal: [][]byte{
					seal1,
				},
			},
		},
	}

	for _, c := range cases {
		data := c.data.MarshalRLPTo(nil)

		ii := &IstanbulExtra{}
		if err := ii.UnmarshalRLP(data); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(c.data, ii) {
			t.Fatal("bad")
		}
	}
}
