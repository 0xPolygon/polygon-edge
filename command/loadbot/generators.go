package loadbot

import (
	"fmt"
	"os"
	"strings"

	"github.com/0xPolygon/polygon-edge/command/loadbot/generator"
	"github.com/0xPolygon/polygon-edge/crypto"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/go-web3/abi"
	"github.com/umbracle/go-web3/jsonrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func createJSONRPCClient(endpoint string, maxConns int) (*jsonrpc.Client, error) {
	client, err := jsonrpc.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create new JSON RPC client: %w", err)
	}

	client.SetMaxConnsLimit(maxConns)

	return client, nil
}

func createGRPCClient(endpoint string) (txpoolOp.TxnPoolOperatorClient, error) {
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return txpoolOp.NewTxnPoolOperatorClient(conn), nil
}

func extractSenderAccount(address types.Address) (*Account, error) {
	sender := &Account{
		Address:    address,
		PrivateKey: nil,
	}

	privateKeyRaw := os.Getenv("LOADBOT_" + address.String())
	privateKeyRaw = strings.TrimPrefix(privateKeyRaw, "0x")
	privateKey, err := crypto.BytesToPrivateKey([]byte(privateKeyRaw))

	if err != nil {
		return nil, fmt.Errorf("failed to extract ECDSA private key from bytes: %w", err)
	}

	sender.PrivateKey = privateKey

	return sender, nil
}

func generateContractArtifactAndArgs(mode Mode) (*generator.ContractArtifact, []byte, error ) {
	var (
		ctrArtifact *generator.ContractArtifact
		ctrArgs []byte
		err error
	)

	if mode == erc20 {
		ctrArtifact = &generator.ContractArtifact{
			Bytecode: ERC20BIN,
			ABI: abi.MustNewABI(ERC20ABI),
		}

		if ctrArgs, err = abi.Encode([]string{"4314500000", "ZexCoin", "ZEX"}, ctrArtifact.ABI.Constructor.Inputs); err != nil {
			return nil, nil , err
		}
	} else if mode == erc721 {
		ctrArtifact = &generator.ContractArtifact{
			Bytecode: ERC721BIN,
			ABI: abi.MustNewABI(ERC721ABI),
		}

		if ctrArgs, err = abi.Encode([]string{"ZEXFT", "ZEXES"}, ctrArtifact.ABI.Constructor.Inputs); err != nil {
			return nil, nil , err
		}
	} else {
		ctrArtifact = &generator.ContractArtifact{
			Bytecode: generator.DefaultContractBytecode,
		}
		ctrArgs = nil
	}

	return ctrArtifact, ctrArgs, nil
}
