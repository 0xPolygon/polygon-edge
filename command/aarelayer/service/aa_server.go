package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	getTransactionReceiptURL = "/v1/getTransactionReceipt/"
	sendTransactionURL       = "/v1/sendTransaction"
)

// AARelayerRestServer represents the service for handling account abstraction transactions
type AARelayerRestServer struct {
	pool         AAPool
	state        AATxState
	verification AAVerification
	server       *http.Server
}

func NewAARelayerRestServer(pool AAPool, state AATxState, verification AAVerification) *AARelayerRestServer {
	return &AARelayerRestServer{
		pool:         pool,
		state:        state,
		verification: verification,
	}
}

// SendTransaction handles the /v1/sendTransaction endpoint
func (s *AARelayerRestServer) sendTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	var tx AATransaction

	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	if err := s.verification.Validate(&tx); err != nil {
		writeMessage(w, http.StatusBadRequest, err.Error())

		return
	}

	// store tx in the state
	stateTx, err := s.state.Add(&tx)
	if err != nil {
		writeMessage(w, http.StatusInternalServerError, err.Error())

		return
	}

	// push state tx to the pool
	s.pool.Push(stateTx)

	writeOutput(w, http.StatusOK, map[string]string{"uuid": stateTx.ID})
}

// GetTransactionReceipt handles the /v1/getTransactionReceipt/{uuid} endpoint
func (s *AARelayerRestServer) getTransactionReceipt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	uuid := r.URL.Path[len(getTransactionReceiptURL):]

	stateTx, err := s.state.Get(uuid)

	if err != nil {
		writeMessage(w, http.StatusInternalServerError, err.Error())

		return
	}

	if stateTx == nil {
		writeMessage(w, http.StatusNotFound, fmt.Sprintf("tx with uuid = %s does not exist", uuid))

		return
	}

	writeOutput(w, http.StatusOK, AAReceipt{
		ID:     uuid,
		Status: stateTx.Status,
		Mined:  stateTx.Mined,
		Error:  stateTx.Error,
		Gas:    stateTx.Gas,
	})
}

func (s *AARelayerRestServer) ListenAndServe(addr string) error {
	s.server = &http.Server{
		Addr: addr,
		// TODO: make this configurable?
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	http.HandleFunc(sendTransactionURL, s.sendTransaction)
	http.HandleFunc(getTransactionReceiptURL, s.getTransactionReceipt)

	return s.server.ListenAndServe()
}

func (s *AARelayerRestServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func writeMessage(w http.ResponseWriter, status int, message string) {
	writeOutput(w, status, map[string]string{"message": message})
}

func writeOutput(w http.ResponseWriter, status int, object interface{}) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(object); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
