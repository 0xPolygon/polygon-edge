package loadbot

import (
	"github.com/umbracle/go-web3/jsonrpc"
)

func shutdownClient(client *jsonrpc.Client) error {
	return client.Close()
}
