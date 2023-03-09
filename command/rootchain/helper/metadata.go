package helper

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

const DefaultPrivateKeyRaw = "aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d"

var (
	ErrRootchainNotFound = errors.New("rootchain not found")
	ErrRootchainPortBind = errors.New("port 8545 is not bind with localhost")

	// rootchainAccountKey is a private key of account which is used for different actions on rootchain
	// (smart contracts deployment, deposits etc.)
	rootchainAccountKey *wallet.Key
)

// InitRootchainPrivateKey initializes a private key instance from provided hex encoded private key
func InitRootchainPrivateKey(rawKey string) error {
	privateKeyRaw := DefaultPrivateKeyRaw
	if rawKey != "" {
		privateKeyRaw = rawKey
	}

	dec, err := hex.DecodeString(privateKeyRaw)
	if err != nil {
		return fmt.Errorf("failed to decode private key string '%s': %w", privateKeyRaw, err)
	}

	rootchainAccountKey, err = wallet.NewWalletFromPrivKey(dec)
	if err != nil {
		return fmt.Errorf("failed to initialize key from provided private key '%s': %w", privateKeyRaw, err)
	}

	return nil
}

// GetRootchainPrivateKey returns rootchain account private key
func GetRootchainPrivateKey() ethgo.Key {
	return rootchainAccountKey
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

// IsTestMode returns true in case provided rootchain private key is the same as DefaultPrivateKey one
func IsTestMode(rootchainPrivKey string) bool {
	return rootchainPrivKey == DefaultPrivateKeyRaw
}
