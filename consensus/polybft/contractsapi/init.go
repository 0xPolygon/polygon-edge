package contractsapi

import (
	"embed"
	"path"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
)

const (
	testContractsDir = "test-contracts"
)

var (
	// core-contracts smart contracts
	CheckpointManager *artifact.Artifact
	ExitHelper        *artifact.Artifact
	L2StateSender     *artifact.Artifact
	StateSender       *artifact.Artifact
	StateReceiver     *artifact.Artifact
	BLS               *artifact.Artifact
	BLS256            *artifact.Artifact
	System            *artifact.Artifact
	ChildValidatorSet *artifact.Artifact
	MRC20             *artifact.Artifact

	// test smart contracts
	//go:embed test-contracts/*
	testContracts          embed.FS
	TestL1StateReceiver    *artifact.Artifact
	TestWriteBlockMetadata *artifact.Artifact
)

func init() {
	var err error

	CheckpointManager, err = artifact.DecodeArtifact([]byte(CheckpointManagerArtifact))
	if err != nil {
		panic(err)
	}

	ExitHelper, err = artifact.DecodeArtifact([]byte(ExitHelperArtifact))
	if err != nil {
		panic(err)
	}

	L2StateSender, err = artifact.DecodeArtifact([]byte(L2StateSenderArtifact))
	if err != nil {
		panic(err)
	}

	BLS, err = artifact.DecodeArtifact([]byte(BLSArtifact))
	if err != nil {
		panic(err)
	}

	BLS256, err = artifact.DecodeArtifact([]byte(BN256G2Artifact))
	if err != nil {
		panic(err)
	}

	StateSender, err = artifact.DecodeArtifact([]byte(StateSenderArtifact))
	if err != nil {
		panic(err)
	}

	StateReceiver, err = artifact.DecodeArtifact([]byte(StateReceiverArtifact))
	if err != nil {
		panic(err)
	}

	System, err = artifact.DecodeArtifact([]byte(SystemArtifact))
	if err != nil {
		panic(err)
	}

	MRC20, err = artifact.DecodeArtifact([]byte(MRC20Artifact))
	if err != nil {
		panic(err)
	}

	ChildValidatorSet, err = artifact.DecodeArtifact([]byte(ChildValidatorSetArtifact))
	if err != nil {
		panic(err)
	}

	testL1StateReceiverRaw, err := testContracts.ReadFile(path.Join(testContractsDir, "TestL1StateReceiver.json"))
	if err != nil {
		panic(err)
	}

	TestL1StateReceiver, err = artifact.DecodeArtifact(testL1StateReceiverRaw)
	if err != nil {
		panic(err)
	}

	testWriteBlockMetadataRaw, err := testContracts.ReadFile(path.Join(testContractsDir, "TestWriteBlockMetadata.json"))
	if err != nil {
		panic(err)
	}

	TestWriteBlockMetadata, err = artifact.DecodeArtifact(testWriteBlockMetadataRaw)
	if err != nil {
		panic(err)
	}
}
