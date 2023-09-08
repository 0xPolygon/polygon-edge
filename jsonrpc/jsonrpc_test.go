package jsonrpc

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/versioning"
)

func TestHTTPServer(t *testing.T) {
	_, err := newTestJSONRPC(t)
	require.NoError(t, err)
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

	require.NoError(
		t,
		json.Unmarshal(mockWriter.Bytes(), response),
	)

	require.Equal(
		t,
		&GetResponse{
			Name:    chainName,
			ChainID: chainID,
			Version: versioning.Version,
		},
		response,
	)
}

func TestJSONRPC_handleJSONRPCRequest(t *testing.T) {
	t.Parallel()

	j, err := newTestJSONRPC(t)
	require.NoError(t, err)

	cases := []struct {
		name             string
		request          string
		expectedResponse string
	}{
		{
			name: "malicious input (XSS attack)",
			request: `{"jsonrpc":"2.0","id":0,"method":"eth_getBlockByNumber","params":["latest",false]}
<script>alert("XSS attack");</script>`,
			expectedResponse: `{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid json request"}}`,
		},
		{
			name:             "valid input",
			request:          `{"jsonrpc":"2.0","id":0,"method":"eth_getBlockByNumber","params":["latest",false]}`,
			expectedResponse: `{"jsonrpc":"2.0","id":0,"result":{`,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("POST", "/eth_getBlockByNumber", strings.NewReader(c.request))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()

			j.handleJSONRPCRequest(w, req)

			response := w.Body.String()
			require.Contains(t, response, c.expectedResponse)
		})
	}
}

func newTestJSONRPC(t *testing.T) (*JSONRPC, error) {
	t.Helper()

	store := newMockStore()

	port, err := tests.GetFreePort()
	require.NoError(t, err, "Unable to fetch free port, %v", err)

	config := &Config{
		Store: store,
		Addr:  &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port},
	}

	return NewJSONRPC(hclog.NewNullLogger(), config)
}
