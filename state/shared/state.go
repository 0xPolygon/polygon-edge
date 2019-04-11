package shared

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	iradix "github.com/hashicorp/go-immutable-radix"
)

type State interface {
	NewTrieAt(common.Hash) (Trie, error)
	NewTrie() Trie
	GetCode(hash common.Hash) ([]byte, bool)
}

type Trie interface {
	Get(k []byte) ([]byte, bool)
	Commit(x *iradix.Tree) (Trie, []byte)
}

// account trie
type accountTrie interface {
	Get(k []byte) ([]byte, bool)
}

// Account is the account reference in the ethereum state
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     common.Hash
	CodeHash []byte
	Trie     accountTrie `rlp:"-"`
}

func (a *Account) String() string {
	return fmt.Sprintf("%d %s", a.Nonce, a.Balance.String())
}

func (a *Account) Copy() *Account {
	aa := new(Account)

	aa.Balance = big.NewInt(1).SetBytes(a.Balance.Bytes())
	aa.Nonce = a.Nonce
	aa.CodeHash = a.CodeHash
	aa.Root = a.Root
	aa.Trie = a.Trie

	return aa
}
