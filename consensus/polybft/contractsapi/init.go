package contractsapi

import (
	"fmt"
	"path"
	"runtime"
)

var (
	Rootchain     *Artifact
	ExitHelper    *Artifact
	L1Exit        *Artifact
	L2StateSender *Artifact
	BLS           *Artifact
	BLS256        *Artifact
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	fmt.Println(filename)
	scpath := path.Join(path.Dir(filename), "../../../core-contracts/artifacts/contracts/")

	var err error
	Rootchain, err = ReadArtifact(scpath, "root/CheckpointManager.sol", "CheckpointManager")
	if err != nil {
		panic(err)
	}
	ExitHelper, err = ReadArtifact(scpath, "root/ExitHelper.sol", "ExitHelper")
	if err != nil {
		panic(err)
	}

	L1Exit, err = ReadArtifact(scpath, "root/L1.sol", "L1")
	if err != nil {
		panic(err)
	}

	L2StateSender, err = ReadArtifact(scpath, "child/L2StateSender.sol", "L2StateSender")
	if err != nil {
		panic(err)
	}

	BLS, err = ReadArtifact(scpath, "common/BLS.sol", "BLS")
	if err != nil {
		panic(err)
	}

	BLS256, err = ReadArtifact(scpath, "common/BN256G2.sol", "BN256G2")
	if err != nil {
		panic(err)
	}

}
