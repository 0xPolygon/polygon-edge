package helper

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

const DefaultPrivateKeyRaw = "aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d"

var (
	// TODO: @Stefan-Ethernal decouple from constants (rely on genesis instead)

	// StateSenderAddress is an address of StateSender.sol smart contract
	StateSenderAddress = types.StringToAddress("0x6FE03c2768C9d800AF3Dedf1878b5687FE120a27")
	// CheckpointManagerAddress is an address of CheckpointManager.sol smart contract
	CheckpointManagerAddress = types.StringToAddress("0x3d46A809D5767B81a8836f0E79145ba615A2Dd61")
	// BLSAddress is an address of BLS.sol smart contract
	BLSAddress = types.StringToAddress("0x72E1C51FE6dABF2e3d5701170cf5aD3620E6B8ba")
	// BN256G2Address is an address of BN256G2Address.sol smart contract
	BN256G2Address = types.StringToAddress("0x436604426F31A05f905C64edc973E575BdB46471")

	ErrRootchainNotFound = errors.New("rootchain not found")
	ErrRootchainPortBind = errors.New("port 8545 is not bind with localhost")

	// rootchainAdminKey is a private key of account which is rootchain administrator
	// namely it represents account which deploys rootchain smart contracts
	rootchainAdminKey *wallet.Key
)

// InitRootchainAdminKey initializes a private key instance from provided hex encoded private key
func InitRootchainAdminKey(rawKey string) error {
	privateKeyRaw := DefaultPrivateKeyRaw
	if rawKey != "" {
		privateKeyRaw = rawKey
	}

	dec, err := hex.DecodeString(privateKeyRaw)
	if err != nil {
		return fmt.Errorf("failed to decode private key string '%s': %w", privateKeyRaw, err)
	}

	rootchainAdminKey, err = wallet.NewWalletFromPrivKey(dec)
	if err != nil {
		return fmt.Errorf("failed to initialize key from provided private key '%s': %w", privateKeyRaw, err)
	}

	return nil
}

// GetRootchainAdminKey returns rootchain admin private key
func GetRootchainAdminKey() ethgo.Key {
	return rootchainAdminKey
}

func GetRootchainID() (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("rootchain id error: %w", err)
	}

	containers, err := cli.ContainerList(context.Background(), dockertypes.ContainerListOptions{})
	if err != nil {
		return "", fmt.Errorf("rootchain id error: %w", err)
	}

	for _, c := range containers {
		if c.Labels["edge-type"] == "rootchain" {
			return c.ID, nil
		}
	}

	return "", ErrRootchainNotFound
}

func ReadRootchainIP() (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("rootchain id error: %w", err)
	}

	contID, err := GetRootchainID()
	if err != nil {
		return "", err
	}

	inspect, err := cli.ContainerInspect(context.Background(), contID)
	if err != nil {
		return "", fmt.Errorf("rootchain ip error: %w", err)
	}

	ports, ok := inspect.HostConfig.PortBindings["8545/tcp"]
	if !ok || len(ports) == 0 {
		return "", ErrRootchainPortBind
	}

	return fmt.Sprintf("http://%s:%s", ports[0].HostIP, ports[0].HostPort), nil
}
