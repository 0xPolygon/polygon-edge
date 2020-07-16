package ibft_genesis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"strconv"
	"time"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/enode"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
	"github.com/spf13/cobra"

	"github.com/0xPolygon/minimal/command"
)

const genesisPath = "chain/chains/ibft.json"
const chainID = 13931
const seal = "0x0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
const istanbulMixHash = "0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365"

var ibftCmd = &cobra.Command{
	Use:   "ibft-genesis",
	Short: "Generates ibft geensis block",
	Run:   ibftRun,
	RunE:  ibftRunE,
}

func init() {
	command.RegisterCmd(ibftCmd)
}

func ibftRun(cmd *cobra.Command, args []string) {
	command.RunCmd(cmd, args, ibftRunE)
}

func ibftRunE(cmd *cobra.Command, args []string) error {
	var validators []types.Address
	var bootnodes []string
	if len(args) < 8 {
		panic("low number of validators")
	}
	if len(args)%2 == 1 {
		panic("provided odd number of args")
	}

	for i := 0; i < len(args); i += 2 {
		key, err := crypto.ParsePrivateKey([]byte(args[i]))
		if err != nil {
			panic(err)
		}

		port, err := strconv.Atoi(args[i+1])
		if err != nil {
			panic(err)
		}

		validator := crypto.PubKeyToAddress(&key.PublicKey)
		validators = append(validators, validator)

		enode := &enode.Enode{
			IP:  net.ParseIP("127.0.0.1"),
			TCP: uint16(port),
			UDP: uint16(port),
			ID:  enode.PubkeyToEnode(&key.PublicKey),
		}

		bootnodes = append(bootnodes, enode.String())
	}

	istanbulExtra := types.IstanbulExtra{
		Validators: validators,
		Seal:       hex.MustDecodeHex(seal),
	}

	w := new(bytes.Buffer)
	err := istanbulExtra.EncodeRLP(w)
	if err != nil {
		panic(err)
	}
	extraData := append(make([]byte, 32), w.Bytes()...)

	c := &chain.Chain{
		Name: "polygon",
		Params: &chain.Params{
			ChainID: chainID,
			Forks: &chain.Forks{
				Homestead: chain.NewFork(0),
				EIP150:    chain.NewFork(0),
				EIP155:    chain.NewFork(0),
				EIP158:    chain.NewFork(0),
			},
			Engine: map[string]interface{}{
				"ibft": map[string]interface{}{
					"epoch":          1000,
					"proposerPolicy": 0,
				},
			},
		},
		Genesis: &chain.Genesis{
			Timestamp:  uint64(time.Now().Unix()),
			ExtraData:  extraData,
			Mixhash:    types.StringToHash(istanbulMixHash),
			GasLimit:   100000000,
			Difficulty: 1,
		},

		Bootnodes: chain.Bootnodes(bootnodes),
	}

	data, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		panic(fmt.Sprintf("Failed to generate genesis: %v", err))
	}

	if err = ioutil.WriteFile(genesisPath, data, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write genesis: %v", err))
	}

	fmt.Printf("Genesis written to %s\n", genesisPath)

	return nil
}
