package rlpx

import (
	"fmt"
	"reflect"
	"testing"
)

func TestMarshalUnmarshalInfo(t *testing.T) {

	info := &Info{
		// Version: 1,
		Name: "mock",
		//ListenPort: 30303,
		Caps: Capabilities{&Cap{"eth", 1}, &Cap{"par", 2}},
		// ID:         enode.PubkeyToEnode(&prv.PublicKey),
	}

	dst := info.MarshalRLP(nil)

	info2 := &Info{}
	if err := info2.UnmarshalRLP(dst); err != nil {
		t.Fatal(err)
	}

	fmt.Println(info)
	fmt.Println(info2)
	fmt.Println(reflect.DeepEqual(info, info2))
}
