package genesis_test

import (
	"encoding/json"
	"github.com/0xPolygon/polygon-sdk/command/util"
	"github.com/mitchellh/cli"
	"io/ioutil"
	"os"
	"strconv"
	"testing"
)

func TestGenesisBlockLimit(t *testing.T) {
	blockLimit := "10000000"
	args := []string{
		"genesis",
		"--blockLimit", blockLimit,
		"--consensus", "ibft",
		"--ibft-validators-prefix-path", "test-chain-",
		"--bootnode", "/ip4/127.0.0.1/tcp/10001/p2p/16Uiu2HAmJxxH1tScDX2rLGSU9exnuvZKNM9SoK3v315azp68DLPW",
	}

	commands := util.Commands()

	cli := &cli.CLI{
		Name:     "polygon",
		Args:     args,
		Commands: commands,
	}

	_, err := cli.Run()
	if err != nil {
		t.Fatalf("cli should have run correctly: %v", err)
	}

	_, err = os.Stat("genesis.json")
	if err != nil {
		t.Fatalf("could not read file info: %v", err)
	}

	type genesisFile struct {
		Genesis struct {
			GasLimit string `json:"gasLimit"`
		} `json:"genesis"`
	}

	file, err := ioutil.ReadFile("genesis.json")
	if err != nil {
		t.Fatalf("could not read file: %v", err)
	}

	data := genesisFile{}
	err = json.Unmarshal(file, &data)
	if err != nil {
		t.Fatalf("failed to unmarshal genesis file into structure: %v", err)
	}

	gotBlockLimit, err := strconv.ParseInt(data.Genesis.GasLimit[2:], 16, 64)
	if err != nil {
		t.Fatalf("failed to convert hexadecimal value to decimal: %v", err)
	}
	expectedBlockLimit, err := strconv.ParseInt(blockLimit, 10, 64)
	if err != nil {
		t.Fatalf("failed to convert block gas limit value to decimal: %v", err)
	}
	if gotBlockLimit != expectedBlockLimit {
		t.Fatalf("invalid block gas limit. expected %d but got %d.", expectedBlockLimit, gotBlockLimit)
	}

	err = os.Remove("genesis.json")
	if err != nil {
		t.Fatalf("failed to remove genesis file: %v", err)
	}
}
