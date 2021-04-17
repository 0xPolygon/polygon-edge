package e2e

import (
	"fmt"
	"testing"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestLogs(t *testing.T) {
	fr := &framework.TestServer{
		Config: &framework.TestServerConfig{
			PremineAccts: []*framework.SrvAccount{
				{
					Addr: types.StringToAddress("0x9bd03347a977e4deb0a0ad685f8385f264524b0b"),
				},
			},
			JsonRPCPort: 8545,
		},
	}

	client := fr.JSONRPC()

	filter := &web3.LogFilter{}
	filter.SetFromUint64(0)
	filter.SetToUint64(5)

	logs, err := client.Eth().GetLogs(filter)
	assert.NoError(t, err)

	fmt.Println(logs)
}

// 0xb8d020ea4c2560106cb3981deb50045717cee853740863e12c524cae3ae6a38a
