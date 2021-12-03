package loadbot

import (
	"context"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/umbracle/go-web3/jsonrpc"
)

func waitForTxPool(endpoint string) error {
	client, err := createGRpcClient(endpoint)
	if err != nil {
		return err
	}

	for {
		status, err := tests.WaitUntilTxPoolEmpty(context.Background(), *client)
		if err != nil {
			return err
		}
		if status.Length == 0 {
			break
		}
	}
	return nil
}

func shutdownClient(client *jsonrpc.Client) error {
	return client.Close()
}
