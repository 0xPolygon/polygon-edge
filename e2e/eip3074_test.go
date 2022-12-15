package e2e

import (
	"context"
	"encoding/json"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/compiler"
	"github.com/umbracle/ethgo/contract"
	"io/ioutil"
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
	sepHash, ok := res["0"].([32]byte)
	require.True(t, ok)
	_ = sepHash

}
