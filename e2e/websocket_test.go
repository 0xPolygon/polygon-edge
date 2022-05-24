package e2e

import (
	"context"
	"encoding/json"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"math/big"
	"testing"
)

type testWSRequest struct {
	JSONRPC string   `json:"jsonrpc"`
	Params  []string `json:"params"`
	Method  string   `json:"method"`
	ID      int      `json:"id"`
}

func constructWSRequest(id int, method string, params []string) ([]byte, error) {
	request := testWSRequest{
		JSONRPC: "2.0",
		Method:  method,
		ID:      id,
		Params:  params,
	}

	return json.Marshal(request)
}

func getWSResponse(t *testing.T, ws *websocket.Conn, request []byte) jsonrpc.SuccessResponse {
	t.Helper()

	if wsError := ws.WriteMessage(websocket.TextMessage, request); wsError != nil {
		t.Fatalf("Unable to write message to WS connection: %v", wsError)
	}

	_, response, wsError := ws.ReadMessage()

	if wsError != nil {
		t.Fatalf("Unable to read message from WS connection: %v", wsError)
	}

	var res jsonrpc.SuccessResponse
	if wsError = json.Unmarshal(response, &res); wsError != nil {
		t.Fatalf("Unable to unmarshal WS response: %v", wsError)
	}

	return res
}

func TestWS_Response(t *testing.T) {
	preminedAccounts := generateTestAccounts(t, 2)
	preminedAccounts[0].balance = framework.EthToWei(10)
	preminedAccounts[1].balance = framework.EthToWei(20)

	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)

		for _, account := range preminedAccounts {
			config.Premine(account.address, account.balance)
		}
	})
	srv := srvs[0]

	// Convert the default JSONRPC address to a WebSocket one
	wsURL := srv.WSJSONRPCURL()

	// Connect to the websocket server
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Unable to connect to WS: %v", err)
	}
	defer ws.Close()

	t.Run("Valid account balance", func(t *testing.T) {
		requestID := 1

		request, constructErr := constructWSRequest(
			requestID,
			"eth_getBalance",
			[]string{preminedAccounts[0].address.String(), "latest"},
		)

		if constructErr != nil {
			t.Fatalf("Unable to construct request: %v", constructErr)
		}

		res := getWSResponse(t, ws, request)

		assert.Equalf(t, res.ID, float64(requestID), "Invalid response ID")

		var balanceHex string
		if wsError := json.Unmarshal(res.Result, &balanceHex); wsError != nil {
			t.Fatalf("Unable to unmarshal WS result: %v", wsError)
		}

		foundBalance, parseError := types.ParseUint256orHex(&balanceHex)
		if parseError != nil {
			t.Fatalf("Unable to parse WS result balance: %v", parseError)
		}

		preminedAccounts[0].balance.Cmp(foundBalance)
		assert.Equalf(t, 0, preminedAccounts[0].balance.Cmp(foundBalance), "Balances don't match")
	})

	t.Run("Valid block number after transfer", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), framework.DefaultTimeout)
		defer cancel()

		_, err = srv.SendRawTx(ctx, &framework.PreparedTransaction{
			From:     preminedAccounts[0].address,
			To:       &preminedAccounts[1].address,
			GasPrice: big.NewInt(10000),
			Gas:      1000000,
			Value:    big.NewInt(10000),
		}, preminedAccounts[0].key)
		if err != nil {
			t.Fatalf("Unable to send transaction, %v", err)
		}

		requestID := 2
		request, constructErr := constructWSRequest(
			requestID,
			"eth_blockNumber",
			[]string{},
		)

		if constructErr != nil {
			t.Fatalf("Unable to construct request: %v", constructErr)
		}

		res := getWSResponse(t, ws, request)

		assert.Equalf(t, res.ID, float64(requestID), "Invalid response ID")

		var blockNum string
		if wsError := json.Unmarshal(res.Result, &blockNum); wsError != nil {
			t.Fatalf("Unable to unmarshal WS result: %v", wsError)
		}

		blockNumInt, parseError := types.ParseUint256orHex(&blockNum)
		if parseError != nil {
			t.Fatalf("Unable to parse WS result balance: %v", parseError)
		}

		assert.Equalf(t, 1, blockNumInt.Cmp(big.NewInt(0)), "Invalid block number")
	})
}
