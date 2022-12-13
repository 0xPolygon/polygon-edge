package itrie

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/umbracle/fastrlp"
)

func TestTrie_Proof(t *testing.T) {
	acct := state.Account{
		Balance: big.NewInt(10),
	}
	val := acct.MarshalWith(&fastrlp.Arena{}).MarshalTo(nil)

	tt := NewTrie()
	txn := tt.Txn()
	txn.Insert([]byte{0x1, 0x2}, val)
	txn.Insert([]byte{0x1, 0x1}, val)

	tracer := &tracer{}
	txn.tracer = tracer

	txn.Hash()
	txn.Lookup([]byte{0x1, 0x2})

	tracer.Proof()
	fmt.Println(tracer.Traces())
}
