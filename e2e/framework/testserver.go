package framework

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"

	"google.golang.org/grpc"

	"github.com/0xPolygon/minimal/minimal/proto"
	txpoolProto "github.com/0xPolygon/minimal/txpool/proto"
)

type TestServerConfigCallback func(*TestServerConfig)

const (
	initialPort   = 12000
	polygonSDKCmd = "polygon-sdk"
)

type TestServer struct {
	t *testing.T

	Config *TestServerConfig
	cmd    *exec.Cmd
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
		JsonRPCPort:   ports[2].Port(),
		RootDir:       rootDir,
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
	return fmt.Sprintf("http://127.0.0.1:%d", t.Config.GRPCPort)
}

func (t *TestServer) JsonRPCAddr() string {
	return fmt.Sprintf("http://127.0.0.1:%d", t.Config.JsonRPCPort)
}

func (t *TestServer) JSONRPC() *jsonrpc.Client {
	clt, err := jsonrpc.NewClient(t.JsonRPCAddr())
	if err != nil {
		t.t.Fatal(err)
	}
	return clt
}

func (t *TestServer) Operator() proto.SystemClient {
	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", t.Config.GRPCPort), grpc.WithInsecure())
	if err != nil {
		t.t.Fatal(err)
	}
	return proto.NewSystemClient(conn)
}

func (t *TestServer) TxnPoolOperator() txpoolProto.TxnPoolOperatorClient {
	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", t.Config.GRPCPort), grpc.WithInsecure())
	if err != nil {
		t.t.Fatal(err)
	}
	return txpoolProto.NewTxnPoolOperatorClient(conn)
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

type InitIBFTResult struct {
	PublicKey string
	NodeID    string
}

func (t *TestServer) InitIBFT() (*InitIBFTResult, error) {
	args := []string{
		"ibft", "init", t.Config.IBFTDir,
	}

	cmd := exec.Command(polygonSDKCmd, args...)
	cmd.Dir = t.Config.RootDir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	res := &InitIBFTResult{}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "Public key:") {
			res.PublicKey = strings.Trim(strings.TrimPrefix(line, "Public Key:"), " ")
		}
		if strings.HasPrefix(line, "Node ID:") {
			res.NodeID = strings.Trim(strings.TrimPrefix(line, "Node ID:"), " ")
		}
	}

	return res, nil
}

func (t *TestServer) GenerateGenesis() error {
	args := []string{
		"genesis",
	}

	// add premines
	for _, acct := range t.Config.PremineAccts {
		args = append(args, "--premine", acct.Addr.String()+":0x"+acct.Balance.Text(16))
	}

	// add consensus flags
	switch t.Config.Consensus {
	case ConsensusIBFT:
		args = append(args, "--consensus", "ibft")
		if t.Config.IBFTDirPrefix == "" {
			return errors.New("prefix of IBFT directory is not set")
		}
		args = append(args, "--ibft-validators-prefix-path", t.Config.IBFTDirPrefix)
		for _, bootnode := range t.Config.Bootnodes {
			args = append(args, "--bootnode", bootnode)
		}
	case ConsensusDev:
		args = append(args, "--consensus", "dev")
	case ConsensusDummy:
		args = append(args, "--consensus", "dummy")
	}

	cmd := exec.Command(polygonSDKCmd, args...)
	cmd.Dir = t.Config.RootDir

	return cmd.Run()
}

func (t *TestServer) Start(ctx context.Context) error {
	args := []string{
		"server",
		// add custom chain
		"--chain", filepath.Join(t.Config.RootDir, "genesis.json"),
		// enable grpc
		"--grpc", fmt.Sprintf(":%d", t.Config.GRPCPort),
		// enable libp2p
		"--libp2p", fmt.Sprintf(":%d", t.Config.LibP2PPort),
		// enable jsonrpc
		"--jsonrpc", fmt.Sprintf(":%d", t.Config.JsonRPCPort),
	}

	switch t.Config.Consensus {
	case ConsensusIBFT:
		args = append(args, "--data-dir", filepath.Join(t.Config.RootDir, t.Config.IBFTDir))
	case ConsensusDev:
		args = append(args, "--data-dir", t.Config.RootDir, "--dev")
	case ConsensusDummy:
		args = append(args, "--data-dir", t.Config.RootDir)
	}

	if t.Config.Seal {
		args = append(args, "--seal")
	}

	// todo: keep this until fix of nat issue
	args = append(args, "--nat", "127.0.0.1")
	//

	if t.Config.ShowsLog {
		args = append(args, "--log-level", "debug")
	}

	t.ReleaseReservedPorts()

	// Start the server
	t.cmd = exec.Command(polygonSDKCmd, args...)
	t.cmd.Dir = t.Config.RootDir

	if t.Config.ShowsLog {
		stdout := io.Writer(os.Stdout)
		t.cmd.Stdout = stdout
		t.cmd.Stderr = stdout
	}

	if err := t.cmd.Start(); err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		for {
			select {
			case <-ctx.Done():
				errCh <- errors.New("timeout")
				return
			default:
				if _, err := t.Operator().GetStatus(context.Background(), &empty.Empty{}); err == nil {
					errCh <- nil
					return
				}
			}
			time.Sleep(time.Second)
		}
	}()

	return <-errCh
}

// DeployContract deploys a contract with account 0 and returns the address
func (t *TestServer) DeployContract(ctx context.Context, binary string) (web3.Address, error) {
	buf, err := hex.DecodeString(binary)
	if err != nil {
		return web3.Address{}, err
	}
	receipt, err := t.SendTxn(ctx, &web3.Transaction{
		Input: buf,
	})
	if err != nil {
		return web3.Address{}, err
	}
	return receipt.ContractAddress, nil
}

const (
	DefaultGasPrice = 1879048192 // 0x70000000
	DefaultGasLimit = 5242880    // 0x500000
)

var emptyAddr web3.Address

func (t *TestServer) SendTxn(ctx context.Context, txn *web3.Transaction) (*web3.Receipt, error) {
	client := t.JSONRPC()

	if txn.From == emptyAddr {
		txn.From = web3.Address(t.Config.PremineAccts[0].Addr)
	}
	if txn.GasPrice == 0 {
		txn.GasPrice = DefaultGasPrice
	}
	if txn.Gas == 0 {
		txn.Gas = DefaultGasLimit
	}
	hash, err := client.Eth().SendTransaction(txn)
	if err != nil {
		return nil, err
	}
	return t.WaitForReceipt(ctx, hash)
}

func (t *TestServer) WaitForReceipt(ctx context.Context, hash web3.Hash) (*web3.Receipt, error) {
	client := t.JSONRPC()

	type getReceiptResult struct {
		receipt *web3.Receipt
		err     error
	}
	resCh := make(chan getReceiptResult, 1)

	go func() {
		defer close(resCh)
		for {
			select {
			case <-ctx.Done():
				resCh <- getReceiptResult{nil, errors.New("timeout")}
				return
			default:
				receipt, err := client.Eth().GetTransactionReceipt(hash)
				if err != nil && err.Error() != "not found" {
					resCh <- getReceiptResult{nil, err}
					return
				}
				if receipt != nil {
					resCh <- getReceiptResult{receipt, nil}
					return
				}
			}
			time.Sleep(time.Second)
		}
	}()
	res := <-resCh
	return res.receipt, res.err
}

func (t *TestServer) WaitForReady(ctx context.Context) error {
	client := t.JSONRPC()

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		for {
			select {
			case <-ctx.Done():
				errCh <- errors.New("timeout")
				return
			default:
				num, err := client.Eth().BlockNumber()
				if err != nil {
					errCh <- err
					return
				}
				if num > 0 {
					errCh <- nil
					return
				}
			}
			time.Sleep(time.Second)
		}
	}()
	return <-errCh
}

func (t *TestServer) TxnTo(ctx context.Context, address web3.Address, method string) *web3.Receipt {
	sig := MethodSig(method)
	receipt, err := t.SendTxn(ctx, &web3.Transaction{
		To:    &address,
		Input: sig,
	})
	if err != nil {
		t.t.Fatal(err)
	}
	return receipt
}
