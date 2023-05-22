package framework

import (
	"net/url"
	"os"
	"testing"

	"github.com/0xPolygon/polygon-edge/command/bridge/common"
	"github.com/0xPolygon/polygon-edge/types"
)

type RemoteBridge struct {
	rpcURL    *url.URL
	binary    string
	senderKey string
}

func NewRemoteBridge(t *testing.T, rpcURL *url.URL, senderKey string) *RemoteBridge {
	t.Helper()

	return &RemoteBridge{
		rpcURL:    rpcURL,
		binary:    "polygon-edge",
		senderKey: senderKey,
	}
}

func (rb *RemoteBridge) Deposit(token common.TokenType,
	rootTokenAddr, rootPredicateAddr types.Address,
	receivers, amounts, tokenIDs string) error {
	return deposit(token, rootTokenAddr, rootPredicateAddr,
		receivers, amounts, tokenIDs,
		rb.binary, os.Stdout, rb.rpcURL, rb.senderKey)
}

func (rb *RemoteBridge) Withdraw(token common.TokenType,
	senderKey, receivers, amounts, tokenIDs, jsonRPCEndpoint string,
	childToken types.Address) error {
	return withdraw(token, senderKey, receivers, amounts, tokenIDs,
		jsonRPCEndpoint, childToken,
		rb.binary, os.Stdout)
}
func (rb *RemoteBridge) SendExitTransaction(exitHelper types.Address,
	exitID uint64,
	rootJSONRPCAddr, childJSONRPCAddr string) error {
	return sendExitTransaction(exitHelper, exitID, rootJSONRPCAddr, childJSONRPCAddr,
		rb.binary, os.Stdout)
}

func (rb *RemoteBridge) JSONRPCAddr() string {
	return rb.rpcURL.String()
}
