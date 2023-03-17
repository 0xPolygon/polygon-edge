package helper

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/sidechain"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

const (
	testAccountPrivKey = "aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d"
	TestModeFlag       = "test"
)

var (
	ErrRootchainNotFound = errors.New("rootchain not found")
	ErrRootchainPortBind = errors.New("port 8545 is not bind with localhost")
	errTestModeSecrets   = errors.New("rootchain test mode does not imply specifying secrets parameters")
)

// GetRootchainTestPrivKey initializes a private key instance from hardcoded test account hex encoded private key
func GetRootchainTestPrivKey() (ethgo.Key, error) {
	testAccPrivKeyRaw, err := hex.DecodeString(testAccountPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key string '%s': %w", testAccountPrivKey, err)
	}

	return wallet.NewWalletFromPrivKey(testAccPrivKeyRaw)
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

// ValidateSecretFlags validates provided secret flags.
// In case isTestMode is set to true, test account is being used and no need to specify secrets,
// otherwise they must be present.
func ValidateSecretFlags(isTestMode bool, accountDir, accountConfigPath string) error {
	if !isTestMode {
		if err := sidechain.ValidateSecretFlags(accountDir, accountConfigPath); err != nil {
			return err
		}
	} else {
		if accountDir != "" || accountConfigPath != "" {
			return errTestModeSecrets
		}
	}

	return nil
}
