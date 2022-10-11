package polybft

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/txpool"
	"github.com/0xPolygon/polygon-edge/types"
)

// polyBFTHeaderHash defines the custom implementation for getting the header hash,
// because of the extraData field
func setupHeaderHashFunc() {
	originalHeaderHash := types.HeaderHash

	types.HeaderHash = func(h *types.Header) types.Hash {
		// when hashing the block for signing we have to remove from
		// the extra field the seal and committed seal items
		extra, err := GetIbftExtraClean(h.ExtraData)
		if err != nil {
			// TODO: this should not happen but we have to handle it somehow
			panic(err)
		}

		// override extra data without seals and committed seal items
		hh := h.Copy()
		hh.ExtraData = extra

		return originalHeaderHash(hh)
	}
}

func AddDummyTx(chainID uint64, key *wallet.Key, txPool *txpool.TxPool, cnt int) {
	for i := 0; i < cnt; i++ {
		to := types.StringToAddress("0x9d4042B5F03C89f76F1963d741d744014acFD21E")
		txn := &types.Transaction{
			From:  types.Address(key.Address()),
			To:    &to,
			V:     big.NewInt(1),
			Value: big.NewInt(10),
			Gas:   21000,
			Nonce: uint64(i),
		}
		txn.ComputeHash()

		signed, err := key.SignTx(chainID, txn)
		if err != nil {
			panic(err)
		}

		err = txPool.AddTx(signed)
		if err != nil {
			panic(err)
		}
	}
}
