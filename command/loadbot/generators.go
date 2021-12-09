package loadbot

import (
	"fmt"
	"os"
	"strings"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3/jsonrpc"
)

func createJsonRpcClient(endpoint string) (*jsonrpc.Client, error) {
	client, err := jsonrpc.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create new JSON RPC client: %v", err)
	}
	return client, nil
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
