package types

import (
	"math/big"

	"github.com/umbracle/fastrlp"
)

const TransactionHashHandler = "TxHash"

type TransactionHashFork interface {
	SerializeForRootCalculation(*Transaction, *fastrlp.ArenaPool) []byte
}

var (
	_ TransactionHashFork = (*TransactionHashForkV1)(nil)
	_ TransactionHashFork = (*TransactionHashForkV2)(nil)
)

type TransactionHashForkV1 struct {
}

func (th *TransactionHashForkV1) SerializeForRootCalculation(t *Transaction, ap *fastrlp.ArenaPool) []byte {
	ar := ap.Get()
	chainID := t.ChainID
	t.ChainID = big.NewInt(0)

	defer func() {
		ap.Put(ar)

		t.ChainID = chainID
	}()

	ar.Reset()

	return t.MarshalRLPWith(ar).MarshalTo(nil)
}

type TransactionHashForkV2 struct {
}

func (th *TransactionHashForkV2) SerializeForRootCalculation(t *Transaction, _ *fastrlp.ArenaPool) []byte {
	return t.MarshalRLPTo(nil)
}
