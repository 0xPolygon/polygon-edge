package jsonrpc

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/versioning"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/go-hclog"
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

func TestHandleJSONRPCRequest_MaliciousInput(t *testing.T) {
	t.Parallel()

	j, err := newTestJSONRPC(t)
	require.NoError(t, err)

	maliciousInput := `<script>alert("XSS attack");</script>`

	req := httptest.NewRequest("POST", "/eth_blockNumber", strings.NewReader(maliciousInput))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	j.handleJSONRPCRequest(w, req)

	responseBody := w.Body.String()
	require.Contains(t, responseBody, "Invalid json request")
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
