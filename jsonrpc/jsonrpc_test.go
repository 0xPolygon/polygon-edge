package jsonrpc

import (
	"bytes"
	"encoding/json"
	"net"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/versioning"
	"github.com/stretchr/testify/assert"

	"github.com/hashicorp/go-hclog"
)

func TestHTTPServer(t *testing.T) {
	store := newMockStore()
	port, portErr := tests.GetFreePort()

	if portErr != nil {
		t.Fatalf("Unable to fetch free port, %v", portErr)
	}

	config := &Config{
		Store: store,
		Addr:  &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port},
	}
	_, err := NewJSONRPC(hclog.NewNullLogger(), config)

	if err != nil {
		t.Fatal(err)
	}
}

func Test_handleGetRequest(t *testing.T) {
	var (
		chainName = "polygon-edge-test"
		chainID   = uint64(200)
	)

	jsonRPC := &JSONRPC{
		config: &Config{
			ChainName: chainName,
			ChainID:   chainID,
		},
	}

	mockWriter := bytes.NewBuffer(nil)

	jsonRPC.handleGetRequest(mockWriter)

	response := &GetResponse{}

	assert.NoError(
		t,
		json.Unmarshal(mockWriter.Bytes(), response),
	)

	assert.Equal(
		t,
		&GetResponse{
			Name:    chainName,
			ChainID: chainID,
			Version: versioning.Version,
		},
		response,
	)
}
