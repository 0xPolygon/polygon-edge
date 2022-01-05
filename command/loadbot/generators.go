package loadbot

import (
	"fmt"
	"github.com/0xPolygon/polygon-sdk/crypto"
	txpoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3/jsonrpc"
	"google.golang.org/grpc"
	"os"
	"strings"
)

func createJsonRpcClient(endpoint string, maxConns int) (*jsonrpc.Client, error) {
	client, err := jsonrpc.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create new JSON RPC client: %v", err)
	}
	client.SetMaxConnsLimit(maxConns)
	return client, nil
}

func createGRPCClient(endpoint string) (txpoolOp.TxnPoolOperatorClient, error) {
	conn, err := grpc.Dial(endpoint, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return txpoolOp.NewTxnPoolOperatorClient(conn), nil
}

func extractSenderAccount(address types.Address) (*Account, error) {
	sender := &Account{
		Address:    address,
		PrivateKey: nil,
	}

	privateKeyRaw := os.Getenv("PSDK_" + address.String())
	privateKeyRaw = strings.TrimPrefix(privateKeyRaw, "0x")
	privateKey, err := crypto.BytesToPrivateKey([]byte(privateKeyRaw))
	if err != nil {
		return nil, fmt.Errorf("failed to extract ECDSA private key from bytes: %v", err)
	}
	sender.PrivateKey = privateKey
	return sender, nil
}
