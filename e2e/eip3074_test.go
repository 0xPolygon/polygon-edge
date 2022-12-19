package e2e

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/invoker"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/compiler"
	"github.com/umbracle/ethgo/contract"
	"io/ioutil"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
)

// cribbing from ethgo
type jsonArtifact struct {
	Bytecode string          `json:"bytecode"`
	Abi      json.RawMessage `json:"abi"`
}

func getTestArtifact(name string) (art *compiler.Artifact, err error) {

	var jsonBytes []byte
	if jsonBytes, err = ioutil.ReadFile(filepath.Join("metadata", name)); err != nil {
		return
	}
	var jart jsonArtifact
	if err = json.Unmarshal(jsonBytes, &jart); err != nil {
		return
	}

	bc := jart.Bytecode
	if !strings.HasPrefix(bc, "0x") {
		bc = "0x" + bc
	}
	art = &compiler.Artifact{
		Abi: string(jart.Abi),
		Bin: bc,
	}
	return
}

type testECDSAKey struct {
	k *ecdsa.PrivateKey
}

func (e *testECDSAKey) Address() ethgo.Address {
	return ethgo.Address(crypto.PubKeyToAddress(&e.k.PublicKey))
}

func (e *testECDSAKey) Sign(hash []byte) ([]byte, error) {
	return crypto.Sign(e.k, hash)
}

var _ ethgo.Key = &testECDSAKey{}

func TestBasicInvoker(t *testing.T) {
	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	receiverKey, _ := tests.GenerateKeyAndAddr(t)

	ibftManager := framework.NewIBFTServersManager(t,
		1,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.Premine(senderAddr, framework.EthToWei(10))
		},
	)

	ctxForStart, cancelStart := context.WithTimeout(context.Background(), 10*framework.DefaultTimeout)
	defer cancelStart()

	ibftManager.StartServers(ctxForStart)

	srv := ibftManager.GetServer(0)

	invokerArt, err := getTestArtifact("AccountAbstractionInvoker.json")
	require.NoError(t, err)
	require.True(t, len(invokerArt.Bin) > 0)

	invokerAbi, err := abi.NewABI(invokerArt.Abi)
	require.NoError(t, err)

	invokerAddr, err := srv.DeployContract(context.Background(), strings.TrimPrefix(invokerArt.Bin, "0x"), senderKey)
	require.NoError(t, err)
	require.NotNil(t, invokerAddr)

	mockArt, err := getTestArtifact("MockContract.json")
	require.NoError(t, err)
	require.True(t, len(invokerArt.Bin) > 0)

	mockAbi, err := abi.NewABI(mockArt.Abi)
	require.NoError(t, err)

	mockAddr, err := srv.DeployContract(context.Background(), strings.TrimPrefix(mockArt.Bin, "0x"), senderKey)
	require.NoError(t, err)
	require.NotNil(t, invokerAddr)

	sk := &testECDSAKey{k: senderKey}
	invokerContract := contract.NewContract(invokerAddr, invokerAbi,
		contract.WithJsonRPC(srv.JSONRPC().Eth()),
		contract.WithSender(sk),
	)

	res, err := invokerContract.Call("DOMAIN_SEPARATOR", ethgo.Latest)
	require.NoError(t, err)
	domainSeparator, ok := res["0"].([32]byte)
	require.True(t, ok)

	tp := invoker.TransactionPayload{
		To:       types.Address(mockAddr),
		Value:    big.NewInt(0),
		GasLimit: big.NewInt(100000),
		Data:     []byte{0xd0, 0x9d, 0xe0, 0x8a}, // const increment = "0xd09de08a";
	}
	res, err = invokerContract.Call("hashPayload", ethgo.Latest, &tp)
	require.NoError(t, err)
	checkHash, ok := res["0"].([32]byte)
	require.True(t, ok)

	th, err := tp.InvokerHash()
	require.NoError(t, err)
	require.Equal(t, checkHash[:], th)

	tps := invoker.TransactionPayloads{tp}
	res, err = invokerContract.Call("hashPayloads", ethgo.Latest, tps.Payloads())
	require.NoError(t, err)
	checkHash, ok = res["0"].([32]byte)
	require.True(t, ok)

	th, err = tps.InvokerHash()
	require.NoError(t, err)
	require.Equal(t, checkHash[:], th)

	k := &testECDSAKey{k: receiverKey}

	it := invoker.InvokerTransaction{
		From:     types.Address(k.Address()),
		Nonce:    big.NewInt(0),
		Payloads: tps,
	}
	res, err = invokerContract.Call("hashTransaction", ethgo.Latest, it)
	require.NoError(t, err)
	checkHash, ok = res["0"].([32]byte)
	require.True(t, ok)

	th, err = it.InvokerHash()
	require.NoError(t, err)
	require.Equal(t, checkHash[:], th)

	res, err = invokerContract.Call("getCommitHash", ethgo.Latest, it)
	require.NoError(t, err)
	checkHash, ok = res["0"].([32]byte)
	require.True(t, ok)

	th, err = it.InvokerCommit(domainSeparator[:])
	require.NoError(t, err)
	require.Equal(t, checkHash[:], th)

	var is invoker.InvokerSignature

	err = is.SignCommit(k, checkHash[:], invokerAddr)
	require.NoError(t, err)

	println(is.String())
	println(hex.EncodeToHex(checkHash[:]))
	println(k.Address().String())
	println(invokerAddr.String())

	invokeTx, err := invokerContract.Txn("invoke", is, it)
	require.NoError(t, err)

	err = invokeTx.Do()
	require.NoError(t, err)
	rcpt, err := invokeTx.Wait()
	require.NoError(t, err)
	require.True(t, rcpt.GasUsed > 0) // whatever ...

	mockContract := contract.NewContract(mockAddr, mockAbi,
		contract.WithJsonRPC(srv.JSONRPC().Eth()),
	)

	res, err = mockContract.Call("lastSender", ethgo.Latest)
	require.NoError(t, err)
	checkAddr, ok := res["0"].(ethgo.Address)
	require.True(t, ok)
	require.Equal(t, k.Address(), checkAddr)

}
