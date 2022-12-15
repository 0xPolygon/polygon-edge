package main

import (
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/dave/jennifer/jen"
)

func main() {
	_, filename, _, _ := runtime.Caller(0) //nolint: dogsled
	currentPath := path.Dir(filename)
	scpath := path.Join(currentPath, "../../../../core-contracts/artifacts/contracts/")

	f := jen.NewFile("contractsapi")
	f.Comment("This is generated file don't change it manually")

	readContracts := []struct {
		Path string
		Name string
	}{
		{
			"root/CheckpointManager.sol",
			"CheckpointManager",
		},
		{
			"root/ExitHelper.sol",
			"ExitHelper",
		},
		{
			"child/L2StateSender.sol",
			"L2StateSender",
		},
		{
			"common/BLS.sol",
			"BLS"},
		{
			"common/BN256G2.sol",
			"BN256G2",
		},
	}

	for _, v := range readContracts {
		artifactBytes, err := artifact.ReadArtifactData(scpath, v.Path, v.Name)
		if err != nil {
			panic(err)
		}

		f.Var().Id(v.Name + "Artifact").String().Op("=").Lit(string(artifactBytes))
	}

	fl, err := os.Create(currentPath + "/../gen_sc_data.go")
	if err != nil {
		panic(err)
	}

	_, err = fmt.Fprintf(fl, "%#v", f)
	if err != nil {
		panic(err)
	}
}
