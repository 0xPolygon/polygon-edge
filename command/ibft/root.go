package ibft

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/minimal/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"

	"github.com/0xPolygon/minimal/command"
)

var ibftCmd = &cobra.Command{
	Use:   "ibft",
	Short: "Calculate ibft extra data.",
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
	if len(args) < 5 {
		panic("low args count")
	}
	for _, arg := range args[:len(args)-1] {
		validators = append(validators, types.StringToAddress(arg))
	}

	istanbulExtra := types.IstanbulExtra{
		Validators: validators,
		Seal:       hexutil.MustDecode(args[len(args)-1]),
	}

	w := new(bytes.Buffer)
	err := istanbulExtra.EncodeRLP(w)
	if err != nil {
		panic(err)
	}

	fmt.Println(hex.EncodeToString(append(make([]byte, 32), w.Bytes()...)))

	return nil
}
