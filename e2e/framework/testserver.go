package framework

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command/genesis/predeploy"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	ibftSwitch "github.com/0xPolygon/polygon-edge/command/ibft/switch"
	initCmd "github.com/0xPolygon/polygon-edge/command/secrets/init"
	"github.com/0xPolygon/polygon-edge/command/server"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/fork"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/local"
	"github.com/0xPolygon/polygon-edge/server/proto"
	txpoolProto "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

type TestServerConfigCallback func(*TestServerConfig)

const (
	serverIP    = "127.0.0.1"
	initialPort = 12000
	binaryName  = "polygon-edge"
)

type TestServer struct {
	t *testing.T

	Config  *TestServerConfig
	cmd     *exec.Cmd
	chainID *big.Int
}

func NewTestServer(t *testing.T, rootDir string, callback TestServerConfigCallback) *TestServer {
	t.Helper()

	// Reserve ports
	ports, err := FindAvailablePorts(3, initialPort, initialPort+10000)
	if err != nil {
		t.Fatal(err)
	}

	// Sets the services to start on open ports
	config := &TestServerConfig{
		ReservedPorts: ports,
		GRPCPort:      ports[0].Port(),
		LibP2PPort:    ports[1].Port(),
		JSONRPCPort:   ports[2].Port(),
		RootDir:       rootDir,
		Signer:        crypto.NewEIP155Signer(100),
		ValidatorType: validators.ECDSAValidatorType,
	}

	if callback != nil {
		callback(config)
	}

	return &TestServer{
		t:      t,
		Config: config,
	}
}

func (t *TestServer) GrpcAddr() string {
	return fmt.Sprintf("%s:%d", serverIP, t.Config.GRPCPort)
}

func (t *TestServer) LibP2PAddr() string {
	return fmt.Sprintf("%s:%d", serverIP, t.Config.LibP2PPort)
}

func (t *TestServer) JSONRPCAddr() string {
	return fmt.Sprintf("%s:%d", serverIP, t.Config.JSONRPCPort)
}

func (t *TestServer) HTTPJSONRPCURL() string {
	return fmt.Sprintf("http://%s", t.JSONRPCAddr())
}

func (t *TestServer) WSJSONRPCURL() string {
	return fmt.Sprintf("ws://%s/ws", t.JSONRPCAddr())
}

func (t *TestServer) JSONRPC() *jsonrpc.Client {
	clt, err := jsonrpc.NewClient(t.HTTPJSONRPCURL())
	if err != nil {
		t.t.Fatal(err)
	}

	return clt
}

func (t *TestServer) Operator() proto.SystemClient {
	conn, err := grpc.Dial(
		t.GrpcAddr(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.t.Fatal(err)
	}

	return proto.NewSystemClient(conn)
}

func (t *TestServer) TxnPoolOperator() txpoolProto.TxnPoolOperatorClient {
	conn, err := grpc.Dial(
		t.GrpcAddr(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.t.Fatal(err)
	}

	return txpoolProto.NewTxnPoolOperatorClient(conn)
}

func (t *TestServer) IBFTOperator() ibftOp.IbftOperatorClient {
	conn, err := grpc.Dial(
		t.GrpcAddr(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.t.Fatal(err)
	}

	return ibftOp.NewIbftOperatorClient(conn)
}

func (t *TestServer) ReleaseReservedPorts() {
	for _, p := range t.Config.ReservedPorts {
		if err := p.Close(); err != nil {
			t.t.Error(err)
		}
	}

	t.Config.ReservedPorts = nil
}

func (t *TestServer) Stop() {
	t.ReleaseReservedPorts()

	if t.cmd != nil {
		if err := t.cmd.Process.Kill(); err != nil {
			t.t.Error(err)
		}
	}
}

func (t *TestServer) GetLatestBlockHeight() (uint64, error) {
	return t.JSONRPC().Eth().BlockNumber()
}

type InitIBFTResult struct {
	Address string
	NodeID  string
}

func (t *TestServer) SecretsInit() (*InitIBFTResult, error) {
	secretsInitCmd := initCmd.GetCommand()

	var args []string

	commandSlice := strings.Split(fmt.Sprintf("secrets %s", secretsInitCmd.Use), " ")
	args = append(args, commandSlice...)
	args = append(args, "--data-dir", filepath.Join(t.Config.IBFTDir, "tmp"))

	cmd := exec.Command(resolveBinary(), args...) //nolint:gosec
	cmd.Dir = t.Config.RootDir

	if _, err := cmd.Output(); err != nil {
		return nil, err
	}

	res := &InitIBFTResult{}

	localSecretsManager, factoryErr := local.SecretsManagerFactory(
		nil,
		&secrets.SecretsManagerParams{
			Logger: hclog.NewNullLogger(),
			Extra: map[string]interface{}{
				secrets.Path: filepath.Join(cmd.Dir, t.Config.IBFTDir),
			},
		})
	if factoryErr != nil {
		return nil, factoryErr
	}

	// Generate the IBFT validator private key
	validatorKey, validatorKeyEncoded, keyErr := crypto.GenerateAndEncodeECDSAPrivateKey()
	if keyErr != nil {
		return nil, keyErr
	}

	// Write the validator private key to the secrets manager storage
	if setErr := localSecretsManager.SetSecret(secrets.ValidatorKey, validatorKeyEncoded); setErr != nil {
		return nil, setErr
	}

	// Generate the libp2p private key
	libp2pKey, libp2pKeyEncoded, keyErr := network.GenerateAndEncodeLibp2pKey()
	if keyErr != nil {
		return nil, keyErr
	}

	// Write the networking private key to the secrets manager storage
	if setErr := localSecretsManager.SetSecret(secrets.NetworkKey, libp2pKeyEncoded); setErr != nil {
		return nil, setErr
	}

	if t.Config.ValidatorType == validators.BLSValidatorType {
		// Generate the BLS Key
		_, bksKeyEncoded, keyErr := crypto.GenerateAndEncodeBLSSecretKey()
		if keyErr != nil {
			return nil, keyErr
		}

		// Write the networking private key to the secrets manager storage
		if setErr := localSecretsManager.SetSecret(secrets.ValidatorBLSKey, bksKeyEncoded); setErr != nil {
			return nil, setErr
		}
	}

	// Get the node ID from the private key
	nodeID, err := peer.IDFromPrivateKey(libp2pKey)
	if err != nil {
		return nil, err
	}

	res.Address = crypto.PubKeyToAddress(&validatorKey.PublicKey).String()
	res.NodeID = nodeID.String()

	return res, nil
}

func (t *TestServer) GenerateGenesis() error {
	genesisCmd := genesis.GetCommand()
	args := []string{
		genesisCmd.Use,
	}

	// add pre-mined accounts
	for _, acct := range t.Config.PremineAccts {
		args = append(args, "--premine", acct.Addr.String()+":0x"+acct.Balance.Text(16))
	}

	// add consensus flags
	switch t.Config.Consensus {
	case ConsensusIBFT:
		args = append(
			args,
			"--consensus", "ibft",
			"--ibft-validator-type", string(t.Config.ValidatorType),
		)

		if t.Config.IBFTDirPrefix == "" {
			return errors.New("prefix of IBFT directory is not set")
		}

		args = append(args, "--ibft-validators-prefix-path", t.Config.IBFTDirPrefix)

		if t.Config.EpochSize != 0 {
			args = append(args, "--epoch-size", strconv.FormatUint(t.Config.EpochSize, 10))
		}

	case ConsensusDev:
		args = append(
			args,
			"--consensus", "dev",
			"--ibft-validator-type", string(t.Config.ValidatorType),
		)

		// Set up any initial staker addresses for the predeployed Staking SC
		for _, stakerAddress := range t.Config.DevStakers {
			args = append(args, "--ibft-validator", stakerAddress.String())
		}
	case ConsensusDummy:
		args = append(args, "--consensus", "dummy")
	}

	for _, bootnode := range t.Config.Bootnodes {
		args = append(args, "--bootnode", bootnode)
	}

	// Make sure the correct mechanism is selected
	if t.Config.IsPos {
		args = append(args, "--pos")

		if t.Config.MinValidatorCount == 0 {
			t.Config.MinValidatorCount = stakingHelper.MinValidatorCount
		}

		if t.Config.MaxValidatorCount == 0 {
			t.Config.MaxValidatorCount = stakingHelper.MaxValidatorCount
		}

		args = append(args, "--min-validator-count", strconv.FormatUint(t.Config.MinValidatorCount, 10))
		args = append(args, "--max-validator-count", strconv.FormatUint(t.Config.MaxValidatorCount, 10))
	}

	// add block gas limit
	if t.Config.BlockGasLimit == 0 {
		t.Config.BlockGasLimit = command.DefaultGenesisGasLimit
	}

	blockGasLimit := strconv.FormatUint(t.Config.BlockGasLimit, 10)
	args = append(args, "--block-gas-limit", blockGasLimit)

	cmd := exec.Command(resolveBinary(), args...) //nolint:gosec
	cmd.Dir = t.Config.RootDir

	stdout := t.GetStdout()
	cmd.Stdout = stdout
	cmd.Stderr = stdout

	return cmd.Run()
}

func (t *TestServer) GenesisPredeploy() error {
	if t.Config.PredeployParams == nil {
		// No need to predeploy anything
		return nil
	}

	genesisPredeployCmd := predeploy.GetCommand()
	args := make([]string, 0)

	commandSlice := strings.Split(fmt.Sprintf("genesis %s", genesisPredeployCmd.Use), " ")

	args = append(args, commandSlice...)

	// Add the path to the genesis file
	args = append(args, "--chain", filepath.Join(t.Config.RootDir, "genesis.json"))

	// Add predeploy address
	if t.Config.PredeployParams.PredeployAddress != "" {
		args = append(args, "--predeploy-address", t.Config.PredeployParams.PredeployAddress)
	}

	// Add constructor arguments, if any
	for _, constructorArg := range t.Config.PredeployParams.ConstructorArgs {
		args = append(args, "--constructor-args", constructorArg)
	}

	// Add the path to the artifacts file
	args = append(args, "--artifacts-path", t.Config.PredeployParams.ArtifactsPath)

	cmd := exec.Command(resolveBinary(), args...) //nolint:gosec
	cmd.Dir = t.Config.RootDir

	stdout := t.GetStdout()
	cmd.Stdout = stdout
	cmd.Stderr = stdout

	return cmd.Run()
}

func (t *TestServer) Start(ctx context.Context) error {
	serverCmd := server.GetCommand()
	args := []string{
		serverCmd.Use,
		// add custom chain
		"--chain", filepath.Join(t.Config.RootDir, "genesis.json"),
		// enable grpc
		"--grpc-address", t.GrpcAddr(),
		// enable libp2p
		"--libp2p", t.LibP2PAddr(),
		// enable jsonrpc
		"--jsonrpc", t.JSONRPCAddr(),
	}

	switch t.Config.Consensus {
	case ConsensusIBFT:
		args = append(args, "--data-dir", filepath.Join(t.Config.RootDir, t.Config.IBFTDir))
	case ConsensusDev:
		args = append(args, "--data-dir", t.Config.RootDir)
		args = append(args, "--dev")

		if t.Config.DevInterval != 0 {
			args = append(args, "--dev-interval", strconv.Itoa(t.Config.DevInterval))
		}
	case ConsensusDummy:
		args = append(args, "--data-dir", t.Config.RootDir)
	}

	if t.Config.PriceLimit != nil {
		args = append(args, "--price-limit", strconv.FormatUint(*t.Config.PriceLimit, 10))
	}

	if t.Config.ShowsLog || t.Config.SaveLogs {
		args = append(args, "--log-level", "debug")
	}

	// add block gas target
	if t.Config.BlockGasTarget != 0 {
		args = append(args, "--block-gas-target", *types.EncodeUint64(t.Config.BlockGasTarget))
	}

	if t.Config.BlockTime != 0 {
		args = append(args, "--block-time", strconv.FormatUint(t.Config.BlockTime, 10))
	}

	if t.Config.IBFTBaseTimeout != 0 {
		args = append(args, "--ibft-base-timeout", strconv.FormatUint(t.Config.IBFTBaseTimeout, 10))
	}

	t.ReleaseReservedPorts()

	// Start the server
	t.cmd = exec.Command(resolveBinary(), args...) //nolint:gosec
	t.cmd.Dir = t.Config.RootDir

	stdout := t.GetStdout()
	t.cmd.Stdout = stdout
	t.cmd.Stderr = stdout

	if err := t.cmd.Start(); err != nil {
		return err
	}

	_, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if _, err := t.Operator().GetStatus(ctx, &empty.Empty{}); err != nil {
			return nil, true
		}

		return nil, false
	})
	if err != nil {
		return err
	}

	// query the chain id
	chainID, err := t.JSONRPC().Eth().ChainID()
	if err != nil {
		return err
	}

	t.chainID = chainID

	return nil
}

func (t *TestServer) SwitchIBFTType(typ fork.IBFTType, from uint64, to, deployment *uint64) error {
	t.t.Helper()

	ibftSwitchCmd := ibftSwitch.GetCommand()
	args := make([]string, 0)

	commandSlice := strings.Split(fmt.Sprintf("ibft %s", ibftSwitchCmd.Use), " ")

	args = append(args, commandSlice...)
	args = append(args,
		// add custom chain
		"--chain", filepath.Join(t.Config.RootDir, "genesis.json"),
		"--type", string(typ),
		"--from", strconv.FormatUint(from, 10),
	)

	// Default ibft validator type for e2e tests is ECDSA
	args = append(args, "--ibft-validator-type", string(validators.ECDSAValidatorType))

	if to != nil {
		args = append(args, "--to", strconv.FormatUint(*to, 10))
	}

	if deployment != nil {
		args = append(args, "--deployment", strconv.FormatUint(*deployment, 10))
	}

	// Start the server
	t.cmd = exec.Command(resolveBinary(), args...) //nolint:gosec
	t.cmd.Dir = t.Config.RootDir

	stdout := t.GetStdout()
	t.cmd.Stdout = stdout
	t.cmd.Stderr = stdout

	return t.cmd.Run()
}

// SignTx is a helper method for signing transactions
func (t *TestServer) SignTx(
	transaction *types.Transaction,
	privateKey *ecdsa.PrivateKey,
) (*types.Transaction, error) {
	return t.Config.Signer.SignTx(transaction, privateKey)
}

// DeployContract deploys a contract with account 0 and returns the address
func (t *TestServer) DeployContract(
	ctx context.Context,
	binary string,
	privateKey *ecdsa.PrivateKey,
) (ethgo.Address, error) {
	buf, err := hex.DecodeString(binary)
	if err != nil {
		return ethgo.Address{}, err
	}

	sender, err := crypto.GetAddressFromKey(privateKey)
	if err != nil {
		return ethgo.ZeroAddress, fmt.Errorf("unable to extract key, %w", err)
	}

	receipt, err := t.SendRawTx(ctx, &PreparedTransaction{
		From:     sender,
		Gas:      DefaultGasLimit,
		GasPrice: big.NewInt(DefaultGasPrice),
		Input:    buf,
	}, privateKey)
	if err != nil {
		return ethgo.Address{}, err
	}

	return receipt.ContractAddress, nil
}

const (
	DefaultGasPrice = 1879048192 // 0x70000000
	DefaultGasLimit = 5242880    // 0x500000
)

type PreparedTransaction struct {
	From     types.Address
	GasPrice *big.Int
	Gas      uint64
	To       *types.Address
	Value    *big.Int
	Input    []byte
}

type Txn struct {
	key     *wallet.Key
	client  *jsonrpc.Eth
	hash    *ethgo.Hash
	receipt *ethgo.Receipt
	raw     *ethgo.Transaction
	chainID *big.Int

	sendErr error
	waitErr error
}

func (t *Txn) Deploy(input []byte) *Txn {
	t.raw.Input = input

	return t
}

func (t *Txn) Transfer(to ethgo.Address, value *big.Int) *Txn {
	t.raw.To = &to
	t.raw.Value = value

	return t
}

func (t *Txn) Value(value *big.Int) *Txn {
	t.raw.Value = value

	return t
}

func (t *Txn) To(to ethgo.Address) *Txn {
	t.raw.To = &to

	return t
}

func (t *Txn) GasLimit(gas uint64) *Txn {
	t.raw.Gas = gas

	return t
}

func (t *Txn) GasPrice(price uint64) *Txn {
	t.raw.GasPrice = price

	return t
}

func (t *Txn) Nonce(nonce uint64) *Txn {
	t.raw.Nonce = nonce

	return t
}

func (t *Txn) sendImpl() error {
	// populate default values
	t.raw.Gas = 1048576
	t.raw.GasPrice = 1048576

	if t.raw.Nonce == 0 {
		nextNonce, err := t.client.GetNonce(t.key.Address(), ethgo.Latest)
		if err != nil {
			return fmt.Errorf("failed to get nonce: %w", err)
		}

		t.raw.Nonce = nextNonce
	}

	signer := wallet.NewEIP155Signer(t.chainID.Uint64())

	signedTxn, err := signer.SignTx(t.raw, t.key)
	if err != nil {
		return err
	}

	data, _ := signedTxn.MarshalRLPTo(nil)

	txHash, err := t.client.SendRawTransaction(data)
	if err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	t.hash = &txHash

	return nil
}

func (t *Txn) Send() *Txn {
	if t.hash != nil {
		panic("BUG: txn already sent")
	}

	t.sendErr = t.sendImpl()

	return t
}

func (t *Txn) Receipt() *ethgo.Receipt {
	return t.receipt
}

//nolint:thelper
func (t *Txn) NoFail(tt *testing.T) {
	t.Wait()

	if t.sendErr != nil {
		tt.Fatal(t.sendErr)
	}

	if t.waitErr != nil {
		tt.Fatal(t.waitErr)
	}

	if t.receipt.Status != 1 {
		tt.Fatal("txn failed with status 0")
	}
}

func (t *Txn) Complete() bool {
	if t.sendErr != nil {
		// txn failed during sending
		return true
	}

	if t.waitErr != nil {
		// txn failed during waiting
		return true
	}

	if t.receipt != nil {
		// txn was mined
		return true
	}

	return false
}

func (t *Txn) Wait() {
	if t.Complete() {
		return
	}

	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()

	receipt, err := tests.WaitForReceipt(ctx, t.client, *t.hash)
	if err != nil {
		t.waitErr = err
	} else {
		t.receipt = receipt
	}
}

func (t *TestServer) Txn(key *wallet.Key) *Txn {
	tt := &Txn{
		key:     key,
		client:  t.JSONRPC().Eth(),
		chainID: t.chainID,
		raw:     &ethgo.Transaction{},
	}

	return tt
}

// SendRawTx signs the transaction with the provided private key, executes it, and returns the receipt
func (t *TestServer) SendRawTx(
	ctx context.Context,
	tx *PreparedTransaction,
	signerKey *ecdsa.PrivateKey,
) (*ethgo.Receipt, error) {
	client := t.JSONRPC()

	nextNonce, err := client.Eth().GetNonce(ethgo.Address(tx.From), ethgo.Latest)
	if err != nil {
		return nil, err
	}

	signedTx, err := t.SignTx(&types.Transaction{
		From:     tx.From,
		GasPrice: tx.GasPrice,
		Gas:      tx.Gas,
		To:       tx.To,
		Value:    tx.Value,
		Input:    tx.Input,
		Nonce:    nextNonce,
	}, signerKey)
	if err != nil {
		return nil, err
	}

	txHash, err := client.Eth().SendRawTransaction(signedTx.MarshalRLP())
	if err != nil {
		return nil, err
	}

	return tests.WaitForReceipt(ctx, t.JSONRPC().Eth(), txHash)
}

func (t *TestServer) WaitForReceipt(ctx context.Context, hash ethgo.Hash) (*ethgo.Receipt, error) {
	client := t.JSONRPC()

	type result struct {
		receipt *ethgo.Receipt
		err     error
	}

	res, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		receipt, err := client.Eth().GetTransactionReceipt(hash)
		if err != nil && err.Error() != "not found" {
			return result{receipt, err}, false
		}
		if receipt != nil {
			return result{receipt, nil}, false
		}

		return nil, true
	})
	if err != nil {
		return nil, err
	}

	data, ok := res.(result)
	if !ok {
		return nil, errors.New("invalid type assertion")
	}

	return data.receipt, data.err
}

// GetGasTotal waits for the total gas used sum for the passed in
// transactions
func (t *TestServer) GetGasTotal(txHashes []ethgo.Hash) uint64 {
	t.t.Helper()

	var (
		totalGasUsed    = uint64(0)
		receiptErrs     = make([]error, 0)
		receiptErrsLock sync.Mutex
		wg              sync.WaitGroup
	)

	appendReceiptErr := func(receiptErr error) {
		receiptErrsLock.Lock()
		defer receiptErrsLock.Unlock()

		receiptErrs = append(receiptErrs, receiptErr)
	}

	for _, txHash := range txHashes {
		wg.Add(1)

		go func(txHash ethgo.Hash) {
			defer wg.Done()

			ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
			defer cancelFn()

			receipt, receiptErr := tests.WaitForReceipt(ctx, t.JSONRPC().Eth(), txHash)
			if receiptErr != nil {
				appendReceiptErr(fmt.Errorf("unable to wait for receipt, %w", receiptErr))

				return
			}

			atomic.AddUint64(&totalGasUsed, receipt.GasUsed)
		}(txHash)
	}

	wg.Wait()

	if len(receiptErrs) > 0 {
		t.t.Fatalf("unable to wait for receipts, %v", receiptErrs)
	}

	return totalGasUsed
}

func (t *TestServer) WaitForReady(ctx context.Context) error {
	_, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		num, err := t.GetLatestBlockHeight()

		if num < 1 || err != nil {
			return nil, true
		}

		return nil, false
	})

	return err
}

func (t *TestServer) InvokeMethod(
	ctx context.Context,
	contractAddress types.Address,
	method string,
	fromKey *ecdsa.PrivateKey,
) *ethgo.Receipt {
	sig := MethodSig(method)

	fromAddress, err := crypto.GetAddressFromKey(fromKey)
	if err != nil {
		t.t.Fatalf("unable to extract key, %v", err)
	}

	receipt, err := t.SendRawTx(ctx, &PreparedTransaction{
		Gas:      DefaultGasLimit,
		GasPrice: big.NewInt(DefaultGasPrice),
		To:       &contractAddress,
		From:     fromAddress,
		Input:    sig,
	}, fromKey)

	if err != nil {
		t.t.Fatal(err)
	}

	return receipt
}

func (t *TestServer) CallJSONRPC(req map[string]interface{}) map[string]interface{} {
	reqJSON, err := json.Marshal(req)
	if err != nil {
		t.t.Fatal(err)

		return nil
	}

	url := fmt.Sprintf("http://%s", t.JSONRPCAddr())

	//nolint:gosec // this is not used because it can't be defined as a global variable
	response, err := http.Post(url, "application/json", bytes.NewReader(reqJSON))
	if err != nil {
		t.t.Fatalf("failed to send request to JSON-RPC server: %v", err)

		return nil
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.t.Fatalf("JSON-RPC doesn't return ok: %s", response.Status)

		return nil
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.t.Fatalf("failed to read HTTP body: %s", err)

		return nil
	}

	result := map[string]interface{}{}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		t.t.Fatalf("failed to convert json to object: %s", err)

		return nil
	}

	return result
}

// GetStdout returns the combined stdout writers of the server
func (t *TestServer) GetStdout() io.Writer {
	writers := []io.Writer{}

	if t.Config.SaveLogs {
		f, err := os.OpenFile(filepath.Join(t.Config.LogsDir, t.Config.Name+".log"), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			t.t.Fatal(err)
		}

		writers = append(writers, f)

		t.t.Cleanup(func() {
			err = f.Close()
			if err != nil {
				t.t.Logf("Failed to close file. Error: %s", err)
			}
		})
	}

	if t.Config.ShowsLog {
		writers = append(writers, os.Stdout)
	}

	if len(writers) == 0 {
		return io.Discard
	}

	return io.MultiWriter(writers...)
}

func resolveBinary() string {
	bin := os.Getenv("EDGE_BINARY")
	if bin != "" {
		return bin
	}
	// fallback
	return binaryName
}
