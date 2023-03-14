package main

import (
	"fmt"
	"log"
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
	f.Comment("This is auto-generated file. DO NOT EDIT.")

	readContracts := []struct {
		Path string
		Name string
	}{
		{
			"child/ChildValidatorSet.sol",
			"ChildValidatorSet",
		},
		{
			"child/L2StateSender.sol",
			"L2StateSender",
		},
		{
			"child/StateReceiver.sol",
			"StateReceiver",
		},
		{
			"child/NativeERC20.sol",
			"NativeERC20",
		},
		{
			"child/ChildERC20.sol",
			"ChildERC20",
		},
		{
			"child/ChildERC20Predicate.sol",
			"ChildERC20Predicate",
		},
		{
			"child/System.sol",
			"System",
		},
		{
			"common/BLS.sol",
			"BLS",
		},
		{
			"common/BN256G2.sol",
			"BN256G2",
		},
		{
			"common/Merkle.sol",
			"Merkle",
		},
		{
			"root/CheckpointManager.sol",
			"CheckpointManager",
		},
		{
			"root/ExitHelper.sol",
			"ExitHelper",
		},
		{
			"root/StateSender.sol",
			"StateSender",
		},
		{
			"root/RootERC20Predicate.sol",
			"RootERC20Predicate",
		},
		{
			"mocks/MockERC20.sol",
			"MockERC20",
		},
	}

	for _, v := range readContracts {
		artifactBytes, err := artifact.ReadArtifactData(scpath, v.Path, v.Name)
		if err != nil {
			log.Fatal(err)
		}

		f.Var().Id(v.Name + "Artifact").String().Op("=").Lit(string(artifactBytes))
	}

	fl, err := os.Create(currentPath + "/../gen_sc_data.go")
	if err != nil {
		log.Fatal(err)
	}

	_, err = fmt.Fprintf(fl, "%#v", f)
	if err != nil {
		log.Fatal(err)
	}
}
