package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/go-hclog"
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
	logger       hclog.Logger
}

func NewAARelayerRestServer(
	pool AAPool, state AATxState, verification AAVerification, logger hclog.Logger) *AARelayerRestServer {
	return &AARelayerRestServer{
		pool:         pool,
		state:        state,
		verification: verification,
		logger:       logger.Named("server"),
	}
}

// SendTransaction handles the /v1/sendTransaction endpoint
func (s *AARelayerRestServer) sendTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMessage(w, http.StatusMethodNotAllowed, fmt.Sprintf("method %s not allowed", r.Method))

		return
	}

	var tx AATransaction

	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		writeMessage(w, http.StatusBadRequest, "failed to decode transaction")

		return
	}

	if err := s.verification.Validate(&tx); err != nil {
		writeMessage(w, http.StatusBadRequest, err.Error())
		s.logger.Error("failed to validate transaction",
			"id", tx.Transaction.From, "nonce", tx.Transaction.Nonce, "err", err)

		return
	}

	// store tx in the state
	stateTx, err := s.state.Add(&tx)
	if err != nil {
		writeMessage(w, http.StatusInternalServerError, err.Error())
		s.logger.Error("failed to add transaction to the state",
			"from", stateTx.Tx.Transaction.From, "nonce", stateTx.Tx.Transaction.Nonce, "err", err)

		return
	}

	// push state tx to the pool
	s.pool.Push(stateTx)

	s.logger.Debug("transaction has been successfully submitted to aa relayer",
		"id", stateTx.ID, "from", stateTx.Tx.Transaction.From, "nonce", stateTx.Tx.Transaction.Nonce)

	writeOutput(w, http.StatusOK, map[string]string{"uuid": stateTx.ID})
}

// GetTransactionReceipt handles the /v1/getTransactionReceipt/{uuid} endpoint
func (s *AARelayerRestServer) getTransactionReceipt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMessage(w, http.StatusMethodNotAllowed, fmt.Sprintf("method %s not allowed", r.Method))

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
		s.logger.Debug("/v1/getTransactionReceipt - transaction does not exist", " id", uuid)

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
		Addr:         addr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	http.HandleFunc(sendTransactionURL, s.sendTransaction)
	http.HandleFunc(getTransactionReceiptURL, s.getTransactionReceipt)

	// Set up CORS middleware.
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set the CORS headers.
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			// If the request method is OPTIONS, return immediately.
			if r.Method == http.MethodOptions {
				return
			}

			// Call the next handler.
			next.ServeHTTP(w, r)
		})
	}

	// Wrap the server's handler with the CORS middleware.
	s.server.Handler = corsMiddleware(http.DefaultServeMux)

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
