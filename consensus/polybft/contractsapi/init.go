package contractsapi

import (
	"embed"
	"fmt"
	"log"
	"path"

	"github.com/0xPolygon/polygon-edge/contracts"
)

const (
	testContractsDir = "test-contracts"
)

var (
	// Blade smart contracts
	CheckpointManager               *contracts.Artifact
	ExitHelper                      *contracts.Artifact
	StateSender                     *contracts.Artifact
	RootERC20Predicate              *contracts.Artifact
	RootERC721Predicate             *contracts.Artifact
	RootERC1155Predicate            *contracts.Artifact
	ChildMintableERC20Predicate     *contracts.Artifact
	ChildMintableERC721Predicate    *contracts.Artifact
	ChildMintableERC1155Predicate   *contracts.Artifact
	BLS                             *contracts.Artifact
	BLS256                          *contracts.Artifact
	System                          *contracts.Artifact
	Merkle                          *contracts.Artifact
	ChildValidatorSet               *contracts.Artifact
	NativeERC20                     *contracts.Artifact
	NativeERC20Mintable             *contracts.Artifact
	StateReceiver                   *contracts.Artifact
	ChildERC20                      *contracts.Artifact
	ChildERC20Predicate             *contracts.Artifact
	ChildERC20PredicateACL          *contracts.Artifact
	RootMintableERC20Predicate      *contracts.Artifact
	RootMintableERC20PredicateACL   *contracts.Artifact
	ChildERC721                     *contracts.Artifact
	ChildERC721Predicate            *contracts.Artifact
	ChildERC721PredicateACL         *contracts.Artifact
	RootMintableERC721Predicate     *contracts.Artifact
	RootMintableERC721PredicateACL  *contracts.Artifact
	ChildERC1155                    *contracts.Artifact
	ChildERC1155Predicate           *contracts.Artifact
	ChildERC1155PredicateACL        *contracts.Artifact
	RootMintableERC1155Predicate    *contracts.Artifact
	RootMintableERC1155PredicateACL *contracts.Artifact
	L2StateSender                   *contracts.Artifact
	CustomSupernetManager           *contracts.Artifact
	StakeManager                    *contracts.Artifact
	EpochManager                    *contracts.Artifact
	RootERC721                      *contracts.Artifact
	RootERC1155                     *contracts.Artifact
	EIP1559Burn                     *contracts.Artifact
	BladeManager                    *contracts.Artifact
	GenesisProxy                    *contracts.Artifact
	TransparentUpgradeableProxy     *contracts.Artifact

	// Governance
	NetworkParams *contracts.Artifact
	ForkParams    *contracts.Artifact
	ChildGovernor *contracts.Artifact
	ChildTimelock *contracts.Artifact

	// test smart contracts
	//go:embed test-contracts/*
	testContracts          embed.FS
	TestWriteBlockMetadata *contracts.Artifact
	RootERC20              *contracts.Artifact
	TestSimple             *contracts.Artifact
	TestRewardToken        *contracts.Artifact

	contractArtifacts map[string]*contracts.Artifact
)

func init() {
	var err error

	CheckpointManager, err = contracts.DecodeArtifact([]byte(CheckpointManagerArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ExitHelper, err = contracts.DecodeArtifact([]byte(ExitHelperArtifact))
	if err != nil {
		log.Fatal(err)
	}

	L2StateSender, err = contracts.DecodeArtifact([]byte(L2StateSenderArtifact))
	if err != nil {
		log.Fatal(err)
	}

	BLS, err = contracts.DecodeArtifact([]byte(BLSArtifact))
	if err != nil {
		log.Fatal(err)
	}

	BLS256, err = contracts.DecodeArtifact([]byte(BN256G2Artifact))
	if err != nil {
		log.Fatal(err)
	}

	Merkle, err = contracts.DecodeArtifact([]byte(MerkleArtifact))
	if err != nil {
		log.Fatal(err)
	}

	StateSender, err = contracts.DecodeArtifact([]byte(StateSenderArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootERC20Predicate, err = contracts.DecodeArtifact([]byte(RootERC20PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootERC721Predicate, err = contracts.DecodeArtifact([]byte(RootERC721PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootERC1155Predicate, err = contracts.DecodeArtifact([]byte(RootERC1155PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildMintableERC20Predicate, err = contracts.DecodeArtifact([]byte(ChildMintableERC20PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildMintableERC721Predicate, err = contracts.DecodeArtifact([]byte(ChildMintableERC721PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildMintableERC1155Predicate, err = contracts.DecodeArtifact([]byte(ChildMintableERC1155PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	StateReceiver, err = contracts.DecodeArtifact([]byte(StateReceiverArtifact))
	if err != nil {
		log.Fatal(err)
	}

	System, err = contracts.DecodeArtifact([]byte(SystemArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC20, err = contracts.DecodeArtifact([]byte(ChildERC20Artifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC20Predicate, err = contracts.DecodeArtifact([]byte(ChildERC20PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC20PredicateACL, err = contracts.DecodeArtifact([]byte(ChildERC20PredicateACLArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootMintableERC20Predicate, err = contracts.DecodeArtifact([]byte(RootMintableERC20PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootMintableERC20PredicateACL, err = contracts.DecodeArtifact([]byte(RootMintableERC20PredicateACLArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC721, err = contracts.DecodeArtifact([]byte(ChildERC721Artifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC721Predicate, err = contracts.DecodeArtifact([]byte(ChildERC721PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC721PredicateACL, err = contracts.DecodeArtifact([]byte(ChildERC721PredicateACLArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootMintableERC721Predicate, err = contracts.DecodeArtifact([]byte(RootMintableERC721PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootMintableERC721PredicateACL, err = contracts.DecodeArtifact([]byte(RootMintableERC721PredicateACLArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC1155, err = contracts.DecodeArtifact([]byte(ChildERC1155Artifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC1155Predicate, err = contracts.DecodeArtifact([]byte(ChildERC1155PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC1155PredicateACL, err = contracts.DecodeArtifact([]byte(ChildERC1155PredicateACLArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootMintableERC1155Predicate, err = contracts.DecodeArtifact([]byte(RootMintableERC1155PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootMintableERC1155PredicateACL, err = contracts.DecodeArtifact([]byte(RootMintableERC1155PredicateACLArtifact))
	if err != nil {
		log.Fatal(err)
	}

	NativeERC20, err = contracts.DecodeArtifact([]byte(NativeERC20Artifact))
	if err != nil {
		log.Fatal(err)
	}

	NativeERC20Mintable, err = contracts.DecodeArtifact([]byte(NativeERC20MintableArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootERC20, err = contracts.DecodeArtifact([]byte(MockERC20Artifact))
	if err != nil {
		log.Fatal(err)
	}

	RootERC721, err = contracts.DecodeArtifact([]byte(MockERC721Artifact))
	if err != nil {
		log.Fatal(err)
	}

	RootERC1155, err = contracts.DecodeArtifact([]byte(MockERC1155Artifact))
	if err != nil {
		log.Fatal(err)
	}

	TestWriteBlockMetadata, err = contracts.DecodeArtifact(readTestContractContent("TestWriteBlockMetadata.json"))
	if err != nil {
		log.Fatal(err)
	}

	TestSimple, err = contracts.DecodeArtifact(readTestContractContent("TestSimple.json"))
	if err != nil {
		log.Fatal(err)
	}

	TestRewardToken, err = contracts.DecodeArtifact(readTestContractContent("TestRewardToken.json"))
	if err != nil {
		log.Fatal(err)
	}

	StakeManager, err = contracts.DecodeArtifact([]byte(StakeManagerArtifact))
	if err != nil {
		log.Fatal(err)
	}

	EpochManager, err = contracts.DecodeArtifact([]byte(EpochManagerArtifact))
	if err != nil {
		log.Fatal(err)
	}

	EIP1559Burn, err = contracts.DecodeArtifact([]byte(EIP1559BurnArtifact))
	if err != nil {
		log.Fatal(err)
	}

	BladeManager, err = contracts.DecodeArtifact([]byte(BladeManagerArtifact))
	if err != nil {
		log.Fatal(err)
	}

	GenesisProxy, err = contracts.DecodeArtifact([]byte(GenesisProxyArtifact))
	if err != nil {
		log.Fatal(err)
	}

	TransparentUpgradeableProxy, err = contracts.DecodeArtifact([]byte(TransparentUpgradeableProxyArtifact))
	if err != nil {
		log.Fatal(err)
	}

	NetworkParams, err = contracts.DecodeArtifact([]byte(NetworkParamsArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ForkParams, err = contracts.DecodeArtifact([]byte(ForkParamsArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildGovernor, err = contracts.DecodeArtifact([]byte(ChildGovernorArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildTimelock, err = contracts.DecodeArtifact([]byte(ChildTimelockArtifact))
	if err != nil {
		log.Fatal(err)
	}

	contractArtifacts = map[string]*contracts.Artifact{
		"CheckpointManager":               CheckpointManager,
		"ExitHelper":                      ExitHelper,
		"StateSender":                     StateSender,
		"RootERC20Predicate":              RootERC20Predicate,
		"RootERC721Predicate":             RootERC721Predicate,
		"RootERC1155Predicate":            RootERC1155Predicate,
		"ChildMintableERC20Predicate":     ChildMintableERC20Predicate,
		"ChildMintableERC721Predicate":    ChildMintableERC721Predicate,
		"ChildMintableERC1155Predicate":   ChildMintableERC1155Predicate,
		"BLS":                             BLS,
		"BLS256":                          BLS256,
		"System":                          System,
		"Merkle":                          Merkle,
		"ChildValidatorSet":               ChildValidatorSet,
		"NativeERC20":                     NativeERC20,
		"NativeERC20Mintable":             NativeERC20Mintable,
		"StateReceiver":                   StateReceiver,
		"ChildERC20":                      ChildERC20,
		"ChildERC20Predicate":             ChildERC20Predicate,
		"ChildERC20PredicateACL":          ChildERC20PredicateACL,
		"RootMintableERC20Predicate":      RootMintableERC20Predicate,
		"RootMintableERC20PredicateACL":   RootMintableERC20PredicateACL,
		"ChildERC721":                     ChildERC721,
		"ChildERC721Predicate":            ChildERC721Predicate,
		"ChildERC721PredicateACL":         ChildERC721PredicateACL,
		"RootMintableERC721Predicate":     RootMintableERC721Predicate,
		"RootMintableERC721PredicateACL":  RootMintableERC721PredicateACL,
		"ChildERC1155":                    ChildERC1155,
		"ChildERC1155Predicate":           ChildERC1155Predicate,
		"ChildERC1155PredicateACL":        ChildERC1155PredicateACL,
		"RootMintableERC1155Predicate":    RootMintableERC1155Predicate,
		"RootMintableERC1155PredicateACL": RootMintableERC1155PredicateACL,
		"L2StateSender":                   L2StateSender,
		"CustomSupernetManager":           CustomSupernetManager,
		"StakeManager":                    StakeManager,
		"EpochManager":                    EpochManager,
		"RootERC721":                      RootERC721,
		"RootERC1155":                     RootERC1155,
		"EIP1559Burn":                     EIP1559Burn,
		"BladeManager":                    BladeManager,
		"GenesisProxy":                    GenesisProxy,
		"TransparentUpgradeableProxy":     TransparentUpgradeableProxy,
		"NetworkParams":                   NetworkParams,
		"ForkParams":                      ForkParams,
		"ChildGovernor":                   ChildGovernor,
		"ChildTimelock":                   ChildTimelock,
		"TestWriteBlockMetadata":          TestWriteBlockMetadata,
		"RootERC20":                       RootERC20,
		"TestSimple":                      TestSimple,
		"TestRewardToken":                 TestRewardToken,
	}
}

func readTestContractContent(contractFileName string) []byte {
	contractRaw, err := testContracts.ReadFile(path.Join(testContractsDir, contractFileName))
	if err != nil {
		log.Fatal(err)
	}

	return contractRaw
}

func GetArtifactFromArtifactName(contractsName string) (*contracts.Artifact, error) {
	if contractArtifact, ok := contractArtifacts[contractsName]; ok {
		return contractArtifact, nil
	}

	return nil, fmt.Errorf("can't find contracts")
}
