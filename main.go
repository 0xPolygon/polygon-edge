package main

import (
	_ "embed"

	"github.com/0xPolygon/polygon-edge/command/root"
	"github.com/0xPolygon/polygon-edge/licenses"

	// Force-load the tracer engines to trigger registration
	_ "github.com/0xPolygon/polygon-edge/tracers/js"
	_ "github.com/0xPolygon/polygon-edge/tracers/native"
)

var (
	//go:embed LICENSE
	license string
)

func main() {
	licenses.SetLicense(license)

	root.NewRootCommand().Execute()
}
