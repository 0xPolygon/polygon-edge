package regenesis

import (
	"github.com/spf13/cobra"
)

var (
	params = &regenesisParams{}
)

type regenesisParams struct {
	TrieDBPath         string
	SnapshotTrieDBPath string
	TrieRoot           string
}

func GetCommand() *cobra.Command {
	genesisCMD := RegenesisCMD()
	genesisCMD.AddCommand(GetRootCMD())
	genesisCMD.AddCommand(HistoryTestCmd())

	return genesisCMD
}
