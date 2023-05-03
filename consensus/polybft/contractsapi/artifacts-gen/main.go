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
			"child/NativeERC20Mintable.sol",
			"NativeERC20Mintable",
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
			"child/ChildERC20PredicateAccessList.sol",
			"ChildERC20PredicateAccessList",
		},
		{
			"child/ChildERC721.sol",
			"ChildERC721",
		},
		{
			"child/ChildERC721Predicate.sol",
			"ChildERC721Predicate",
		},
		{
			"child/ChildERC721PredicateAccessList.sol",
			"ChildERC721PredicateAccessList",
		},
		{
			"child/ChildERC1155.sol",
			"ChildERC1155",
		},
		{
			"child/ChildERC1155Predicate.sol",
			"ChildERC1155Predicate",
		},
		{
			"child/ChildERC1155PredicateAccessList.sol",
			"ChildERC1155PredicateAccessList",
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
			"mocks/MockERC20.sol",
			"MockERC20",
		},
		{
			"root/RootERC20Predicate.sol",
			"RootERC20Predicate",
		},
		{
			"mocks/MockERC721.sol",
			"MockERC721",
		},
		{
			"root/RootERC721Predicate.sol",
			"RootERC721Predicate",
		},
		{
			"mocks/MockERC1155.sol",
			"MockERC1155",
		},
		{
			"root/RootERC1155Predicate.sol",
			"RootERC1155Predicate",
		},
		{
			"root/staking/CustomSupernetManager.sol",
			"CustomSupernetManager",
		},
		{
			"root/staking/StakeManager.sol",
			"StakeManager",
		},
		{
			"child/validator/RewardPool.sol",
			"RewardPool",
		},
		{
			"child/validator/ValidatorSet.sol",
			"ValidatorSet",
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
