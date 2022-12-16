package e2e

import (
	"context"
	"encoding/json"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
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

func TestBasicInvoker(t *testing.T) {
	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	_, receiverAddr := tests.GenerateKeyAndAddr(t)

	_ = receiverAddr

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

	invokerContract := contract.NewContract(invokerAddr, invokerAbi, contract.WithJsonRPC(srv.JSONRPC().Eth()))

	res, err := invokerContract.Call("DOMAIN_SEPARATOR", ethgo.Latest)
	require.NoError(t, err)
	domainSeparator, ok := res["0"].([32]byte)
	require.True(t, ok)

	tp := TransactionPayload{
		To:       types.Address{0xde, 0xad, 0xbe, 0xef},
		Value:    big.NewInt(1000),
		GasLimit: big.NewInt(1000),
		Data:     []byte{0x00, 0x01, 0x02, 0x03},
	}
	res, err = invokerContract.Call("hashPayload", ethgo.Latest, &tp)
	require.NoError(t, err)
	checkHash, ok := res["0"].([32]byte)
	require.True(t, ok)

	th, err := tp.InvokerHash()
	require.NoError(t, err)
	require.Equal(t, checkHash[:], th)

	tps := TransactionPayloads{tp}
	res, err = invokerContract.Call("hashPayloads", ethgo.Latest, tps.Payloads())
	require.NoError(t, err)
	checkHash, ok = res["0"].([32]byte)
	require.True(t, ok)

	th, err = tps.InvokerHash()
	require.NoError(t, err)
	require.Equal(t, checkHash[:], th)

	it := InvokerTransaction{
		From:     types.Address{0xca, 0xfe, 0xba, 0xbe},
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

}

type InvokerTransaction struct {
	From     types.Address        `abi:"from"`
	Nonce    *big.Int             `abi:"nonce"`
	Payloads []TransactionPayload `abi:"payloads"`
}

// return keccak256(abi.encode(TRANSACTION_TYPE, transaction.from, transaction.nonce, hashPayloads(transaction.payloads)));

type encTransaction struct {
	TypeHash     [32]byte      `abi:"typeHash"`
	From         types.Address `abi:"from"`
	Nonce        *big.Int      `abi:"nonce"`
	PayloadsHash [32]byte      `abi:"payloadsHash"`
}

var transactionType = ethgo.Keccak256([]byte("Transaction(address from,uint256 nonce,TransactionPayload[] payloads)TransactionPayload(address to,uint256 value,uint256 gasLimit,bytes data)"))

func (it InvokerTransaction) InvokerCommit(domainSeparator []byte) (c []byte, err error) {
	values := []byte{0x19, 0x01}
	values = append(values, domainSeparator...)
	if c, err = it.InvokerHash(); err != nil {
		return
	}
	values = append(values, c...)
	c = ethgo.Keccak256(values)
	return
}

func (it InvokerTransaction) InvokerHash() (h []byte, err error) {

	var t *abi.Type
	if t, err = abi.NewType("tuple(bytes32 typeHash, address from, uint256 nonce, bytes32 payloadsHash)"); err != nil {
		return
	}

	enc := encTransaction{
		From:  it.From,
		Nonce: it.Nonce,
	}
	copy(enc.TypeHash[:], transactionType)
	tps := TransactionPayloads(it.Payloads)
	if h, err = tps.InvokerHash(); err != nil {
		return
	}
	copy(enc.PayloadsHash[:], h)

	var encBytes []byte
	if encBytes, err = abi.Encode(&enc, t); err != nil {
		return
	}

	h = ethgo.Keccak256(encBytes)
	return

}

type TransactionPayloads []TransactionPayload

func (tps TransactionPayloads) InvokerHash() (h []byte, err error) {
	var values []byte
	for i := range tps {
		if h, err = tps[i].InvokerHash(); err != nil {
			return
		}
		values = append(values, h...)
	}
	// return keccak256(abi.encodePacked(values));
	h = ethgo.Keccak256(values)
	return
}

func (tps TransactionPayloads) Payloads() []TransactionPayload {
	return tps
}

type TransactionPayload struct {
	To       types.Address `abi:"to"`
	Value    *big.Int      `abi:"value"`
	GasLimit *big.Int      `abi:"gasLimit"`
	Data     []byte        `abi:"data"`
}

type encPayload struct {
	TypeHash [32]byte      `abi:"typeHash"`
	To       types.Address `abi:"to"`
	Value    *big.Int      `abi:"value"`
	GasLimit *big.Int      `abi:"gasLimit"`
	DataHash [32]byte      `abi:"dataHash"`
}

// from AccountAbstractionInvoker.sol: TRANSACTION_PAYLOAD_TYPE
var transactionPayloadType = ethgo.Keccak256([]byte("TransactionPayload(address to,uint256 value,uint256 gasLimit,bytes data)"))

func (tp TransactionPayload) InvokerHash() (h []byte, err error) {

	var t *abi.Type
	if t, err = abi.NewType("tuple(bytes32 typeHash, address to, uint256 value, uint256 gasLimit, bytes32 dataHash)"); err != nil {
		return
	}

	enc := encPayload{
		To:       tp.To,
		Value:    tp.Value,
		GasLimit: tp.GasLimit,
	}
	copy(enc.TypeHash[:], transactionPayloadType)
	copy(enc.DataHash[:], ethgo.Keccak256(tp.Data))

	var encBytes []byte
	if encBytes, err = abi.Encode(&enc, t); err != nil {
		return
	}

	h = ethgo.Keccak256(encBytes)
	return
}
