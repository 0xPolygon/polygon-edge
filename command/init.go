package command

import (
	"flag"
	"fmt"
	"net"
	"path/filepath"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/keystore"
	"github.com/0xPolygon/minimal/minimal"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mitchellh/cli"
)

// InitCommand is the command to show the version of the agent
type InitCommand struct {
	UI cli.Ui
}

// Help implements the cli.Command interface
func (c *InitCommand) Help() string {
	return ""
}

// Synopsis implements the cli.Command interface
func (c *InitCommand) Synopsis() string {
	return ""
}

// Run implements the cli.Command interface
func (c *InitCommand) Run(args []string) int {
	var dataDir string
	var libp2pAddrStr string

	flags := flag.NewFlagSet("genesis", flag.ContinueOnError)
	flags.StringVar(&dataDir, "data-dir", "test-chain-1", "")
	flags.StringVar(&libp2pAddrStr, "libp2p", "", "")

	if err := flags.Parse(args); err != nil {
		c.UI.Error(fmt.Sprintf("Failed to parse args: %v", err))
		return 1
	}

	if dataDir == "" {
		c.UI.Error("data dir is empty")
		return 1
	}

	if err := minimal.SetupDataDir(dataDir, []string{"keystore"}); err != nil {
		c.UI.Error(fmt.Sprintf("failed to create data dir: %v", err))
		return 1
	}

	keystorePath := filepath.Join(dataDir, "keystore")
	key, err := keystore.ReadPrivKey(filepath.Join(keystorePath, "key"))
	if err != nil {
		c.UI.Error(fmt.Sprintf("failed with priv key: %v", err))
		return 1
	}

	libp2pKey, err := keystore.ReadLibp2pKey(filepath.Join(keystorePath, "libp2p"))
	if err != nil {
		c.UI.Error(fmt.Sprintf("failed with libp2p key: %v", err))
		return 1
	}

	address := crypto.PubKeyToAddress(&key.PublicKey)
	fmt.Printf("Seal key address: %s\n", address.String())

	pid, err := peer.IDFromPublicKey(libp2pKey.GetPublic())
	if err != nil {
		c.UI.Error(fmt.Sprintf("failed to create peer id %v", err))
		return 1
	}

	netAddr, err := net.ResolveTCPAddr("tcp", libp2pAddrStr)
	if err != nil {
		c.UI.Error(fmt.Sprintf("failed to resolve libp2p addr %s: %v", libp2pAddrStr, err))
		return 1
	}
	if netAddr.IP == nil {
		netAddr.IP = net.ParseIP("127.0.0.1")
	}

	libp2pAddr := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", netAddr.IP.String(), netAddr.Port, pid)
	fmt.Printf("Libp2p address: %s\n", libp2pAddr)
	return 0
}
