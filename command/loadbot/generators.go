package loadbot

import (
	"fmt"
	"github.com/0xPolygon/polygon-sdk/crypto"
	txPoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3/jsonrpc"
	"google.golang.org/grpc"
	"os"
)

func createJsonRpcClient(endpoint string) (*jsonrpc.Client, error) {
	client, err := jsonrpc.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create new JSON RPC client: %v", err)
	}
	return client, nil
}

func createGRpcClient(endpoint string) (*txPoolOp.TxnPoolOperatorClient, error) {
	conn, err := grpc.Dial(endpoint, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %v", err)
	}

	client := txPoolOp.NewTxnPoolOperatorClient(conn)
	return &client, nil
}

func extractSenderAccount(address types.Address) (*Account, error) {
	sender := &Account{
		Address:    address,
		PrivateKey: nil,
	}

	privateKeyRaw := os.Getenv("PSDK_" + address.String())
	privateKeyRaw = privateKeyRaw[2:]
	privateKey, err := crypto.BytesToPrivateKey([]byte(privateKeyRaw))
	if err != nil {
		return nil, fmt.Errorf("failed to extract ECDSA private key from bytes: %v", err)
	}
	sender.PrivateKey = privateKey
	return sender, nil
}
