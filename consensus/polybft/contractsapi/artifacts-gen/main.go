package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"log"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/helper/common"
)

const (
	extension = ".sol"
)

func main() {
	_, filename, _, _ := runtime.Caller(0) //nolint: dogsled
	currentPath := path.Dir(filename)
	scpath := path.Join(currentPath, "../../../../core-contracts/artifacts/contracts/")

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
			"ChildERC20PredicateACL",
		},
		{
			"child/RootMintableERC20Predicate.sol",
			"RootMintableERC20Predicate",
		},
		{
			"child/RootMintableERC20PredicateAccessList.sol",
			"RootMintableERC20PredicateACL",
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
			"ChildERC721PredicateACL",
		},
		{
			"child/RootMintableERC721Predicate.sol",
			"RootMintableERC721Predicate",
		},
		{
			"child/RootMintableERC721PredicateAccessList.sol",
			"RootMintableERC721PredicateACL",
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
			"ChildERC1155PredicateACL",
		},
		{
			"child/RootMintableERC1155Predicate.sol",
			"RootMintableERC1155Predicate",
		},
		{
			"child/RootMintableERC1155PredicateAccessList.sol",
			"RootMintableERC1155PredicateACL",
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
			"root/ChildMintableERC20Predicate.sol",
			"ChildMintableERC20Predicate",
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
			"root/ChildMintableERC721Predicate.sol",
			"ChildMintableERC721Predicate",
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
			"root/ChildMintableERC1155Predicate.sol",
			"ChildMintableERC1155Predicate",
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
		{
			"child/EIP1559Burn.sol",
			"EIP1559Burn",
		},
		{
			"lib/GenesisProxy.sol",
			"GenesisProxy",
		},
		{
			"../@openzeppelin/contracts/proxy/transparent/TransparentUpgradeableProxy.sol",
			"TransparentUpgradeableProxy",
		},
	}

	str := `// This is auto-generated file. DO NOT EDIT.
package contractsapi

`

	for _, v := range readContracts {
		artifactBytes, err := artifact.ReadArtifactData(scpath, v.Path, getContractName(v.Path))
		if err != nil {
			log.Fatal(err)
		}

		dst := &bytes.Buffer{}
		if err = json.Compact(dst, artifactBytes); err != nil {
			log.Fatal(err)
		}

		str += fmt.Sprintf("var %sArtifact string = `%s`\n", v.Name, dst.String())
	}

	output, err := format.Source([]byte(str))
	if err != nil {
		fmt.Println(str)
		log.Fatal(err)
	}

	if err = common.SaveFileSafe(currentPath+"/../gen_sc_data.go", output, 0600); err != nil {
		log.Fatal(err)
	}
}

// getContractName extracts smart contract name from provided path
func getContractName(path string) string {
	pathSegments := strings.Split(path, string([]rune{os.PathSeparator}))
	nameSegment := pathSegments[len(pathSegments)-1]

	return strings.Split(nameSegment, extension)[0]
}
