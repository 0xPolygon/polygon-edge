package main

import (
	_ "embed"

	"github.com/dogechain-lab/jury/command/root"
	"github.com/dogechain-lab/jury/licenses"
)

var (
	//go:embed LICENSE
	license string
)

func main() {
	licenses.SetLicense(license)

	root.NewRootCommand().Execute()
}
