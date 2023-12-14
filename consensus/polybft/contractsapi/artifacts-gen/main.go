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
	scpath := path.Join(currentPath, "../../../../blade-contracts/artifacts/contracts/")

	readContracts := []struct {
		Path string
		Name string
	}{
		{
			"blade/L2StateSender.sol",
			"L2StateSender",
		},
		{
			"blade/StateReceiver.sol",
			"StateReceiver",
		},
		{
			"blade/NativeERC20.sol",
			"NativeERC20",
		},
		{
			"blade/NativeERC20Mintable.sol",
			"NativeERC20Mintable",
		},
		{
			"blade/ChildERC20.sol",
			"ChildERC20",
		},
		{
			"blade/ChildERC20Predicate.sol",
			"ChildERC20Predicate",
		},
		{
			"blade/ChildERC20PredicateAccessList.sol",
			"ChildERC20PredicateACL",
		},
		{
			"blade/RootMintableERC20Predicate.sol",
			"RootMintableERC20Predicate",
		},
		{
			"blade/RootMintableERC20PredicateAccessList.sol",
			"RootMintableERC20PredicateACL",
		},
		{
			"blade/ChildERC721.sol",
			"ChildERC721",
		},
		{
			"blade/ChildERC721Predicate.sol",
			"ChildERC721Predicate",
		},
		{
			"blade/ChildERC721PredicateAccessList.sol",
			"ChildERC721PredicateACL",
		},
		{
			"blade/RootMintableERC721Predicate.sol",
			"RootMintableERC721Predicate",
		},
		{
			"blade/RootMintableERC721PredicateAccessList.sol",
			"RootMintableERC721PredicateACL",
		},
		{
			"blade/ChildERC1155.sol",
			"ChildERC1155",
		},
		{
			"blade/ChildERC1155Predicate.sol",
			"ChildERC1155Predicate",
		},
		{
			"blade/ChildERC1155PredicateAccessList.sol",
			"ChildERC1155PredicateACL",
		},
		{
			"blade/RootMintableERC1155Predicate.sol",
			"RootMintableERC1155Predicate",
		},
		{
			"blade/RootMintableERC1155PredicateAccessList.sol",
			"RootMintableERC1155PredicateACL",
		},
		{
			"blade/System.sol",
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
			"bridge/CheckpointManager.sol",
			"CheckpointManager",
		},
		{
			"bridge/ExitHelper.sol",
			"ExitHelper",
		},
		{
			"bridge/StateSender.sol",
			"StateSender",
		},
		{
			"mocks/MockERC20.sol",
			"MockERC20",
		},
		{
			"bridge/RootERC20Predicate.sol",
			"RootERC20Predicate",
		},
		{
			"bridge/ChildMintableERC20Predicate.sol",
			"ChildMintableERC20Predicate",
		},
		{
			"mocks/MockERC721.sol",
			"MockERC721",
		},
		{
			"bridge/RootERC721Predicate.sol",
			"RootERC721Predicate",
		},
		{
			"bridge/ChildMintableERC721Predicate.sol",
			"ChildMintableERC721Predicate",
		},
		{
			"mocks/MockERC1155.sol",
			"MockERC1155",
		},
		{
			"bridge/RootERC1155Predicate.sol",
			"RootERC1155Predicate",
		},
		{
			"bridge/ChildMintableERC1155Predicate.sol",
			"ChildMintableERC1155Predicate",
		},
		{
			"blade/staking/StakeManager.sol",
			"StakeManager",
		},
		{
			"blade/validator/EpochManager.sol",
			"EpochManager",
		},
		{
			"blade/EIP1559Burn.sol",
			"EIP1559Burn",
		},
		{
			"bridge/BladeManager.sol",
			"BladeManager",
		},
		{
			"lib/GenesisProxy.sol",
			"GenesisProxy",
		},
		{
			"../@openzeppelin/contracts/proxy/transparent/TransparentUpgradeableProxy.sol",
			"TransparentUpgradeableProxy",
		},
		{
			"blade/NetworkParams.sol",
			"NetworkParams",
		},
		{
			"blade/ForkParams.sol",
			"ForkParams",
		},
		{
			"blade/governance/ChildGovernor.sol",
			"ChildGovernor",
		},
		{
			"blade/governance/ChildTimelock.sol",
			"ChildTimelock",
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
