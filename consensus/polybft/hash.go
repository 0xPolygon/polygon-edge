package polybft

import (
	"encoding/hex"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/crypto"
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

func AddDummyTx(chianId uint64, txPool *txpool.TxPool, cnt int) {
	// GENERATE account and encode to string, premine account and change constant
	// Account is 0x9d4042B5F03C89f76F1963d741d744014acFD20F
	// go run ./main.go genesis-polybft --block-gas-limit 10000000 --validator-set-size=4 --premine 0x9d4042B5F03C89f76F1963d741d744014acFD20F:1000000000000000000000
	// account := wallet.GenerateAccount()
	// fmt.Println(account.Ecdsa.Address().String())
	// c, _ := account.ToBytes()
	// fmt.Println(hex.EncodeToString(c))

	const encodedAccount = "7b226563647361223a2263373365363838333561656666643635383632623939386161666234623961393663353234643731323566653338626362343538643465613464303530326265222c22626c73223a2233313338333633393336333133363336333833333339333533353330333633313333333233343333333833383335333533333339333033323336333633373331333233333333333233333339333333313337333533373330333133383338333733353332333133393336333433363331333533303336333933313334333133303333333633323337333733373336333533343332333333393333227d"

	bytes, _ := hex.DecodeString(encodedAccount)
	account, _ := wallet.NewAccountFromBytes(bytes)

	for i := 0; i < cnt; i++ {
		to := types.StringToAddress("0x9d4042B5F03C89f76F1963d741d744014acFD21E")
		txn := &types.Transaction{
			From:  types.Address(account.Ecdsa.Address()),
			To:    &to,
			V:     big.NewInt(1),
			Value: big.NewInt(10),
			Gas:   21000,
			Nonce: uint64(i),
		}
		txn.ComputeHash()

		signer := crypto.NewEIP155Signer(chianId)
		pk, _ := account.GetEcdsaPrivateKey()
		txn, _ = signer.SignTx(txn, pk)

		err := txPool.AddTx(txn)
		if err != nil {
			panic(err)
		}
	}
}
