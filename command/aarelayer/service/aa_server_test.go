package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

const (
	baseURL = "127.0.0.1:8289"
	chainID = 100
)

var dummySignature, _ = hex.DecodeString("41b5bb4e1eab25b9d8f09f40d8ce91fa8c8d23084ee9a533841cbe42074eb7cc2f0674a1c5abba9d0cf05aeb39f340dcaf42aff68b3e63a8d5cf7514dcd940e600")

func Test_AAServer(t *testing.T) {
	t.Parallel()

	dbpath, err := os.MkdirTemp("", "aa_server_state_db")
	require.NoError(t, err)

	defer os.RemoveAll(dbpath)

	userAccount, err := wallet.GenerateAccount()
	require.NoError(t, err)

	aaServer := getServer(t, aaInvokerAddress, dbpath)

	go func() {
		err := aaServer.ListenAndServe(baseURL)
		require.True(t, err == nil || errors.Is(err, http.ErrServerClosed))
	}()

	t.Cleanup(func() {
		require.NoError(t, aaServer.Shutdown(context.Background()))
	})

	time.Sleep(time.Millisecond * 100) // wait for server to start

	t.Run("sendTransaction_getTransactionReceipt_ok", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{}
		tx := &AATransaction{
			Signature: dummySignature,
			Transaction: Transaction{
				From:  types.Address(userAccount.Ecdsa.Address()),
				Nonce: 0,
				Payload: []Payload{
					{
						To:       &types.Address{1, 2, 3},
						Value:    big.NewInt(10),
						GasLimit: big.NewInt(21000),
					},
					{
						To:       nil,
						Value:    big.NewInt(100),
						GasLimit: big.NewInt(21000),
					},
				},
			},
		}

		require.NoError(t, tx.MakeSignature(aaInvokerAddress, chainID, userAccount.Ecdsa))

		require.Equal(t, tx.Transaction.From, tx.GetAddressFromSignature(aaInvokerAddress, chainID))

		req := makeRequest(t, "POST", "sendTransaction", tx)

		res, err := client.Do(req)
		require.NoError(t, err)

		// Check that the response code is 200 OK
		require.Equal(t, http.StatusOK, res.StatusCode)

		// Check that the response body contains the expected data
		uuidBytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		var responseObj map[string]string

		require.NoError(t, json.Unmarshal(uuidBytes, &responseObj))

		uuid := responseObj["uuid"]

		require.True(t, len(uuid) > 10)

		req = makeRequest(t, "GET", fmt.Sprintf("getTransactionReceipt/%s", uuid), nil)

		res, err = client.Do(req)
		require.NoError(t, err)

		// Check that the response code is 200 OK
		require.Equal(t, http.StatusOK, res.StatusCode)

		// Check that the response body contains the expected data
		bytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		receipt := AAReceipt{}

		require.NoError(t, json.Unmarshal(bytes, &receipt))

		require.Equal(t, uuid, receipt.ID)
	})

	t.Run("sendTransaction_WrongMethod", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{}
		req := makeRequest(t, "GET", "sendTransaction", &AATransaction{})

		res, err := client.Do(req)
		require.NoError(t, err)

		require.Equal(t, http.StatusMethodNotAllowed, res.StatusCode)
	})

	t.Run("getTransactionReceipt_WrongMethod", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{}
		req := makeRequest(t, "POST", "getTransactionReceipt/321", nil)

		res, err := client.Do(req)
		require.NoError(t, err)

		require.Equal(t, http.StatusMethodNotAllowed, res.StatusCode)
	})

	t.Run("getTransactionReceipt_TxUUIDNotExist", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{}
		req := makeRequest(t, "GET", "getTransactionReceipt/321", nil)

		res, err := client.Do(req)
		require.NoError(t, err)

		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("sendTransaction_WrongInput", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{}

		req := makeRequest(t, "POST", "sendTransaction", nil)

		res, err := client.Do(req)
		require.NoError(t, err)

		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("sendTransaction_EmptyPayload", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{}

		req := makeRequest(t, "POST", "sendTransaction", &AATransaction{})

		res, err := client.Do(req)
		require.NoError(t, err)

		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("sendTransaction_EmptySignature", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{}
		tx := &AATransaction{
			Transaction: Transaction{
				Nonce:   0,
				Payload: []Payload{{}},
			},
		}

		for _, sig := range [][]byte{nil, {}, {1}} {
			tx.Signature = sig
			req := makeRequest(t, "POST", "sendTransaction", &tx)

			res, err := client.Do(req)
			require.NoError(t, err)

			require.Equal(t, http.StatusBadRequest, res.StatusCode)
		}
	})

	t.Run("sendTransaction_WrongFrom", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{}
		tx := &AATransaction{
			Signature: dummySignature,
			Transaction: Transaction{
				Nonce: 0,
				Payload: []Payload{
					{
						To:       &types.Address{1, 2, 3},
						Value:    big.NewInt(100),
						GasLimit: big.NewInt(21000),
					},
				},
			},
		}

		require.NoError(t, tx.MakeSignature(aaInvokerAddress, chainID, userAccount.Ecdsa))

		req := makeRequest(t, "POST", "sendTransaction", &tx)

		res, err := client.Do(req)
		require.NoError(t, err)

		require.Equal(t, http.StatusBadRequest, res.StatusCode)

		tx.Transaction.From = types.Address{1, 2}
		req = makeRequest(t, "POST", "sendTransaction", &tx)

		res, err = client.Do(req)
		require.NoError(t, err)

		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("sendTransaction_EmptyValue", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{}
		tx := &AATransaction{
			Signature: dummySignature,
			Transaction: Transaction{
				Nonce: 0,
				Payload: []Payload{
					{
						To:       &types.Address{1, 2, 3},
						GasLimit: big.NewInt(21000),
						Value:    nil,
					},
				},
			},
		}

		req := makeRequest(t, "POST", "sendTransaction", &tx)

		res, err := client.Do(req)
		require.NoError(t, err)

		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("sendTransaction_EmptyGasLimit", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{}
		tx := &AATransaction{
			Signature: dummySignature,
			Transaction: Transaction{
				Nonce: 0,
				Payload: []Payload{
					{
						To: &types.Address{1, 2, 3},
					},
				},
			},
		}

		req := makeRequest(t, "POST", "sendTransaction", &tx)

		res, err := client.Do(req)
		require.NoError(t, err)

		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})
}

func getServer(t *testing.T, address types.Address, dbpath string) *AARelayerRestServer {
	t.Helper()

	state, err := NewAATxState(path.Join(dbpath, "relayer.db"))
	require.NoError(t, err)

	config := DefaultConfig()
	pool := NewAAPool()
	verification := NewAAVerification(config, address, chainID, func(a *AATransaction) error {
		return nil
	})

	config.DenyList = []string{types.ZeroAddress.String()}

	return NewAARelayerRestServer(pool, state, verification, hclog.NewNullLogger())
}

func makeRequest(t *testing.T, httpMethod, endpoint string, obj interface{}) *http.Request {
	t.Helper()

	var body io.Reader

	if obj != nil {
		var buf bytes.Buffer

		require.NoError(t, json.NewEncoder(&buf).Encode(obj))

		body = &buf
	}

	// Create a new request to the endpoint
	req, err := http.NewRequest(httpMethod, fmt.Sprintf("http://%s/v1/%s", baseURL, endpoint), body)
	require.NoError(t, err)

	return req
}
