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
		"ibft",
		"init",
		t.Config.IBFTDir,
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
	case ConsensusDummy:
		args = append(args, "--consensus", "dummy")
	}

	cmd := exec.Command(polygonSDKCmd, args...)
	cmd.Dir = t.Config.RootDir

	return cmd.Run()
}

func (t *TestServer) Start() error {
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
	case ConsensusDummy:
		args = append(args, "--data-dir", t.Config.RootDir)
	}

	if t.Config.Seal {
		args = append(args, "--seal")
	}
	if t.Config.DevMode {
		args = append(args, "--dev")
	}

	// todo: keep this until fix of nat issue
	args = append(args, "--nat", "127.0.0.1")
	args = append(args, "--log-level", "debug")
	//

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
		t.t.Fatalf("err: %s", err)
	}

	// todo: timeout
	for {
		if _, err := t.Operator().GetStatus(context.Background(), &empty.Empty{}); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	return nil
}

// DeployContract deploys a contract with account 0 and returns the address
func (t *TestServer) DeployContract(binary string) (web3.Address, error) {
	buf, err := hex.DecodeString(binary)
	if err != nil {
		return web3.Address{}, err
	}
	receipt, err := t.SendTxn(&web3.Transaction{
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

func (t *TestServer) SendTxn(txn *web3.Transaction) (*web3.Receipt, error) {
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
	return t.WaitForReceipt(hash)
}

func (t *TestServer) WaitForReceipt(hash web3.Hash) (*web3.Receipt, error) {
	client := t.JSONRPC()

	var receipt *web3.Receipt
	var count uint64
	var err error

	for {
		receipt, err = client.Eth().GetTransactionReceipt(hash)
		if err != nil {
			if err.Error() != "not found" {
				return nil, err
			}
		}
		if receipt != nil {
			break
		}
		if count > 5 {
			return nil, fmt.Errorf("timeout")
		}
		time.Sleep(1 * time.Second)
		count++
	}
	return receipt, nil
}

func (t *TestServer) WaitForReady() error {
	client := t.JSONRPC()

	count := 0
	for {
		num, err := client.Eth().BlockNumber()
		if err != nil {
			return err
		}
		if num > 0 {
			break
		}

		if count++; count > 5 {
			return errors.New("timeout")
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

func (t *TestServer) TxnTo(address web3.Address, method string) *web3.Receipt {
	sig := MethodSig(method)
	receipt, err := t.SendTxn(&web3.Transaction{
		To:    &address,
		Input: sig,
	})
	if err != nil {
		t.t.Fatal(err)
	}
	return receipt
}
