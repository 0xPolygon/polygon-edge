package e2e

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/invoker"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/compiler"
	"github.com/umbracle/ethgo/contract"
)

// cribbing from ethgo
type jsonArtifact struct {
	Bytecode string          `json:"bytecode"`
	Abi      json.RawMessage `json:"abi"`
}

func getTestArtifact(name string) (art *compiler.Artifact, err error) {

	var jsonBytes []byte
	if jsonBytes, err = os.ReadFile(filepath.Join("metadata", name)); err != nil {
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
			config.SetConsensus(framework.ConsensusDev)
		},
	)

	ctxForStart, cancelStart := context.WithTimeout(context.Background(), 10*framework.DefaultTimeout)
	defer cancelStart()

	ibftManager.StartServers(ctxForStart)

	srv := ibftManager.GetServer(0)

	deployArtifact := func(name string, withKey *ecdsa.PrivateKey, args []interface{}) (*contract.Contract, ethgo.Address) {
		art, err := getTestArtifact(name)
		require.NoError(t, err)
		require.True(t, len(art.Bin) > 0)

		theAbi, err := abi.NewABI(art.Abi)
		require.NoError(t, err)

		binStr := strings.TrimPrefix(art.Bin, "0x")
		if theAbi.Constructor != nil && theAbi.Constructor.Inputs != nil && len(args) > 0 {

			constructorArgs, err := abi.Encode(args, theAbi.Constructor.Inputs)
			require.NoError(t, err)

			binary, err := hex.DecodeString(binStr)
			require.NoError(t, err)

			binary = append(binary, constructorArgs...)
			binStr = hex.EncodeToString(binary)
		}

		addr, err := srv.DeployContract(context.Background(), binStr, withKey)
		require.NoError(t, err)
		require.NotNil(t, addr)

		sk := &testECDSAKey{k: withKey}
		artifact := contract.NewContract(addr, theAbi,
			contract.WithJsonRPC(srv.JSONRPC().Eth()),
			contract.WithSender(sk),
		)

		return artifact, addr
	}

	invokerContract, invokerAddr := deployArtifact("AccountAbstractionInvoker.json", senderKey, nil)
	mockContract, mockAddr := deployArtifact("MockContract.json", senderKey, nil)

	res, err := invokerContract.Call("DOMAIN_SEPARATOR", ethgo.Latest)
	require.NoError(t, err)
	domainSeparator, ok := res["0"].([32]byte)
	require.True(t, ok)

	tp := invoker.TransactionPayload{
		To:       types.Address(mockAddr),
		Value:    big.NewInt(0),
		GasLimit: big.NewInt(100000),
		Data:     framework.MethodSig("increment"),
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

	testReceiverKey := &testECDSAKey{k: receiverKey}
	it := invoker.InvokerTransaction{
		From:     types.Address(testReceiverKey.Address()),
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

	err = is.SignCommit(testReceiverKey, checkHash[:], invokerAddr)
	require.NoError(t, err)

	invokeTx, err := invokerContract.Txn("invoke", is, it)
	require.NoError(t, err)

	err = invokeTx.Do()
	require.NoError(t, err)
	rcpt, err := invokeTx.Wait()
	require.NoError(t, err)
	require.True(t, rcpt.GasUsed > 0) // whatever ...

	res, err = mockContract.Call("lastSender", ethgo.Latest)
	require.NoError(t, err)
	checkAddr, ok := res["0"].(ethgo.Address)
	require.True(t, ok)
	require.Equal(t, testReceiverKey.Address(), checkAddr)

	res, err = mockContract.Call("counter", ethgo.Latest)
	require.NoError(t, err)
	checkCounter, ok := res["0"].(*big.Int)
	require.True(t, ok)
	require.Equal(t, big.NewInt(1), checkCounter)

	// do same with account session invoker

	// whitelisted contracts
	allowed := []types.Address{types.Address(mockAddr.Address())}
	args := []interface{}{allowed}

	sessionInvokerContract, sessionInvokerAddr := deployArtifact("AccountSessionInvoker.json", senderKey, args)

	res, err = sessionInvokerContract.Call("isWhitelisted", ethgo.Latest, mockAddr)
	require.NoError(t, err)
	isWhitelisted, ok := res["0"].(bool)
	require.True(t, ok)
	require.True(t, isWhitelisted)

	sessionToken := invoker.SessionToken{
		Delegate:   senderAddr,
		Expiration: big.NewInt(0),
	}

	res, err = sessionInvokerContract.Call("hashSessionToken", ethgo.Latest, sessionToken)
	require.NoError(t, err)
	checkHash, ok = res["0"].([32]byte)
	require.True(t, ok)

	sessionTokenHash, err := sessionToken.InvokerHash()
	require.NoError(t, err)
	require.Equal(t, checkHash[:], sessionTokenHash)

	res, err = sessionInvokerContract.Call("getCommitHash", ethgo.Latest, sessionToken)
	require.NoError(t, err)
	checkHash, ok = res["0"].([32]byte)
	require.True(t, ok)

	var invokerSignature invoker.InvokerSignature
	err = invokerSignature.SignCommit(testReceiverKey, checkHash[:], sessionInvokerAddr)
	require.NoError(t, err)

	sessionTx, err := sessionInvokerContract.Txn("invoke", invokerSignature, sessionToken, tps.Payloads())
	require.NoError(t, err)

	err = sessionTx.Do()
	require.NoError(t, err)
	rcpt, err = sessionTx.Wait()
	require.NoError(t, err)

	res, err = mockContract.Call("counter", ethgo.Latest)
	require.NoError(t, err)
	checkCounter, ok = res["0"].(*big.Int)
	require.True(t, ok)
	require.Equal(t, big.NewInt(2), checkCounter)
}
