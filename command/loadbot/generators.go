package loadbot

import (
	"fmt"
	"os"
	"strings"

	"github.com/0xPolygon/polygon-edge/crypto"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/jsonrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func createJSONRPCClient(endpoint string, maxConns int) (*jsonrpc.Client, error) {
	client, err := jsonrpc.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create new JSON RPC client: %w", err)
	}

	client.SetMaxConnsLimit(maxConns)

	return client, nil
}

func createGRPCClient(endpoint string) (txpoolOp.TxnPoolOperatorClient, error) {
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
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

	privateKeyRaw := os.Getenv("LOADBOT_" + address.String())
	privateKeyRaw = strings.TrimPrefix(privateKeyRaw, "0x")
	privateKey, err := crypto.BytesToECDSAPrivateKey([]byte(privateKeyRaw))

	if err != nil {
		return nil, fmt.Errorf("failed to extract ECDSA private key from bytes: %w", err)
	}

	sender.PrivateKey = privateKey

	return sender, nil
}
