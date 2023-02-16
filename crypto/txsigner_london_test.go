package crypto

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/btcsuite/btcd/btcec"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestLondonSigner_Sender(t *testing.T) {
	t.Parallel()

	toAddress := types.StringToAddress("1")

	testTable := []struct {
		name    string
		chainID *big.Int
	}{
		{
			"mainnet",
			big.NewInt(1),
		},
		{
			"expanse mainnet",
			big.NewInt(2),
		},
		{
			"ropsten",
			big.NewInt(3),
		},
		{
			"rinkeby",
			big.NewInt(4),
		},
		{
			"goerli",
			big.NewInt(5),
		},
		{
			"kovan",
			big.NewInt(42),
		},
		{
			"geth private",
			big.NewInt(1337),
		},
		{
			"mega large",
			big.NewInt(0).Exp(big.NewInt(2), big.NewInt(20), nil), // 2**20
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			key, keyGenError := GenerateECDSAKey()
			if keyGenError != nil {
				t.Fatalf("Unable to generate key")
			}

			txn := &types.Transaction{
				To:       &toAddress,
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(0),
			}

			chainID := testCase.chainID.Uint64()
			signer := NewLondonSigner(chainID, NewEIP155Signer(chainID))

			signedTx, signErr := signer.SignTx(txn, key)
			if signErr != nil {
				t.Fatalf("Unable to sign transaction")
			}

			recoveredSender, recoverErr := signer.Sender(signedTx)
			if recoverErr != nil {
				t.Fatalf("Unable to recover sender")
			}

			assert.Equal(t, recoveredSender.String(), PubKeyToAddress(&key.PublicKey).String())
		})
	}
}

func Test_extract_sender(t *testing.T) {
	signer := NewLondonSigner(100, NewEIP155Signer(100))
	to := types.StringToAddress("0xDeaDbeefdEAdbeefdEadbEEFdeadbeEFdEaDbeeF")
	r, ok := big.NewInt(0).SetString("98797584628888384160758764785165211431852976435563509556022205469260415465959", 10)
	if !ok {
		t.Fatal("not ok")
	}
	s, ok := big.NewInt(0).SetString("38161463957492767083580775540579543089314424059727471367316340605410967185213", 10)
	if !ok {
		t.Fatal("not ok")
	}

	tx := &types.Transaction{
		Type:      types.DynamicFeeTx,
		Nonce:     0,
		GasPrice:  big.NewInt(1000000402),
		GasTipCap: big.NewInt(1000000000),
		GasFeeCap: big.NewInt(10000000000),
		Gas:       21000,
		To:        &to,
		Value:     big.NewInt(100000000000000),
		Input:     nil,
		V:         big.NewInt(1),
		R:         r,
		S:         s,
	}

	pk, err := hex.DecodeString("42b6e34dc21598a807dc19d7784c71b2a7a01f6480dc6f58258f78e539f1a1fa")
	if err != nil {
		t.Fatal(err)
	}

	prv, _ := btcec.PrivKeyFromBytes(S256, pk)

	singedTx, err := signer.SignTx(tx, prv.ToECDSA())
	if err != nil {
		t.Fatal(err)
	}

	sender, err := signer.Sender(tx)
	if err != nil {
		t.Fatal(err)
	}

	senderSigned, err := signer.Sender(singedTx)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("R", singedTx.R)
	fmt.Println("S", singedTx.S)
	fmt.Println("V", singedTx.V)

	// 0x85dA99c8a7C2C95964c8EfD687E95E632Fc533D6
	fmt.Println("sender", sender)
	fmt.Println("senderSigned", senderSigned)
}
