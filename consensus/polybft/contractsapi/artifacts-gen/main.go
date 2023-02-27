package main

import (
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/dave/jennifer/jen"
)

type contractInfo struct {
	Path string
	Name string
}

func main() {
	_, filename, _, _ := runtime.Caller(0) //nolint: dogsled
	currentPath := path.Dir(filename)

	f := jen.NewFile("contractsapi")
	f.Comment("This is auto-generated file. DO NOT EDIT.")

	populateContracts(
		f,
		path.Join(currentPath, "../../../../core-contracts/artifacts/contracts/"),
		[]contractInfo{
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
		})

	populateContracts(
		f,
		path.Join(currentPath, "../../../../account-abstraction-invoker/artifacts/contracts/"),
		[]contractInfo{
			{
				"AccountAbstractionInvoker.sol",
				"AccountAbstractionInvoker",
			},
		})

	fl, err := os.Create(currentPath + "/../gen_sc_data.go")
	if err != nil {
		panic(err)
	}

	_, err = fmt.Fprintf(fl, "%#v", f)
	if err != nil {
		panic(err)
	}
}

func populateContracts(f *jen.File, scpath string, contractInfos []contractInfo) {
	for _, v := range contractInfos {
		artifactBytes, err := artifact.ReadArtifactData(scpath, v.Path, v.Name)
		if err != nil {
			panic(err)
		}

		f.Var().Id(v.Name + "Artifact").String().Op("=").Lit(string(artifactBytes))
	}
}
