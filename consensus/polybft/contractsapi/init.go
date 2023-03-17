package contractsapi

import (
	"embed"
	"log"
	"path"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
)

const (
	testContractsDir = "test-contracts"
)

var (
	// core-contracts smart contracts
	CheckpointManager   *artifact.Artifact
	ExitHelper          *artifact.Artifact
	StateSender         *artifact.Artifact
	RootERC20Predicate  *artifact.Artifact
	BLS                 *artifact.Artifact
	BLS256              *artifact.Artifact
	System              *artifact.Artifact
	Merkle              *artifact.Artifact
	ChildValidatorSet   *artifact.Artifact
	NativeERC20         *artifact.Artifact
	StateReceiver       *artifact.Artifact
	ChildERC20          *artifact.Artifact
	ChildERC20Predicate *artifact.Artifact
	L2StateSender       *artifact.Artifact

	// test smart contracts
	//go:embed test-contracts/*
	testContracts          embed.FS
	TestL1StateReceiver    *artifact.Artifact
	TestWriteBlockMetadata *artifact.Artifact
	RootERC20              *artifact.Artifact
)

func init() {
	var err error

	CheckpointManager, err = artifact.DecodeArtifact([]byte(CheckpointManagerArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ExitHelper, err = artifact.DecodeArtifact([]byte(ExitHelperArtifact))
	if err != nil {
		log.Fatal(err)
	}

	L2StateSender, err = artifact.DecodeArtifact([]byte(L2StateSenderArtifact))
	if err != nil {
		log.Fatal(err)
	}

	BLS, err = artifact.DecodeArtifact([]byte(BLSArtifact))
	if err != nil {
		log.Fatal(err)
	}

	BLS256, err = artifact.DecodeArtifact([]byte(BN256G2Artifact))
	if err != nil {
		log.Fatal(err)
	}

	Merkle, err = artifact.DecodeArtifact([]byte(MerkleArtifact))
	if err != nil {
		log.Fatal(err)
	}

	StateSender, err = artifact.DecodeArtifact([]byte(StateSenderArtifact))
	if err != nil {
		log.Fatal(err)
	}

	RootERC20Predicate, err = artifact.DecodeArtifact([]byte(RootERC20PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	StateReceiver, err = artifact.DecodeArtifact([]byte(StateReceiverArtifact))
	if err != nil {
		log.Fatal(err)
	}

	System, err = artifact.DecodeArtifact([]byte(SystemArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC20, err = artifact.DecodeArtifact([]byte(ChildERC20Artifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildERC20Predicate, err = artifact.DecodeArtifact([]byte(ChildERC20PredicateArtifact))
	if err != nil {
		log.Fatal(err)
	}

	ChildValidatorSet, err = artifact.DecodeArtifact([]byte(ChildValidatorSetArtifact))
	if err != nil {
		log.Fatal(err)
	}

	NativeERC20, err = artifact.DecodeArtifact([]byte(NativeERC20Artifact))
	if err != nil {
		log.Fatal(err)
	}

	RootERC20, err = artifact.DecodeArtifact([]byte(MockERC20Artifact))
	if err != nil {
		log.Fatal(err)
	}

	TestL1StateReceiver, err = artifact.DecodeArtifact(readTestContractContent("TestL1StateReceiver.json"))
	if err != nil {
		log.Fatal(err)
	}

	TestWriteBlockMetadata, err = artifact.DecodeArtifact(readTestContractContent("TestWriteBlockMetadata.json"))
	if err != nil {
		log.Fatal(err)
	}
}

func readTestContractContent(contractFileName string) []byte {
	contractRaw, err := testContracts.ReadFile(path.Join(testContractsDir, contractFileName))
	if err != nil {
		log.Fatal(err)
	}

	return contractRaw
}
