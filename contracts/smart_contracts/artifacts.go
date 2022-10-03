package smartcontracts

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/umbracle/ethgo/abi"

	"github.com/0xPolygon/polygon-edge/types"
)

//go:embed artifacts/*
var artifactsDir embed.FS

func MustReadArtifact(chain string, name string) *Artifact {
	artifact, err := ReadArtifact(chain, name)
	if err != nil {
		panic(err)
	}

	return artifact
}

func ReadArtifact(chain string, name string) (*Artifact, error) {
	if chain != "sidechain" && chain != "rootchain" {
		return nil, fmt.Errorf("chain has to be either 'rootchain' or 'sidechain'")
	}

	fileName := "artifacts/contracts/" + chain + "/" + name + ".sol/" + name + ".json"

	data, err := artifactsDir.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	var hexRes struct {
		Abi              *abi.ABI
		Bytecode         string
		DeployedBytecode string
	}

	if err = json.Unmarshal(data, &hexRes); err != nil {
		return nil, fmt.Errorf("artifact found but no correct format: %v", err)
	}

	return &Artifact{
		Abi:              hexRes.Abi,
		Bytecode:         types.StringToBytes(hexRes.Bytecode),
		DeployedBytecode: types.StringToBytes(hexRes.DeployedBytecode),
	}, nil
}

type Artifact struct {
	Abi              *abi.ABI
	Bytecode         []byte
	DeployedBytecode []byte
}

func (a *Artifact) DeployInput(args []interface{}) ([]byte, error) {
	input := []byte{}
	input = append(input, a.Bytecode...)

	if a.Abi.Constructor != nil {
		argsInput, err := abi.Encode(args, a.Abi.Constructor.Inputs)
		if err != nil {
			return nil, err
		}

		input = append(input, argsInput...)
	}

	return input, nil
}
