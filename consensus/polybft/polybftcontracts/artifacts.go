package polybftcontracts

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/umbracle/ethgo/abi"
)

func ReadArtifact(rootFolder, contractPath, contractName string) (*Artifact, error) {
	fileName := filepath.Join(rootFolder, contractPath, fmt.Sprintf("%s.json", contractName))

	absolutePath, err := filepath.Abs(fileName)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(absolutePath)
	if err != nil {
		return nil, err
	}

	var hexRes struct {
		Abi              *abi.ABI
		Bytecode         string
		DeployedBytecode string
	}

	if err = json.Unmarshal(data, &hexRes); err != nil {
		return nil, fmt.Errorf("artifact found but no correct format: %w", err)
	}

	return &Artifact{
		Abi:              hexRes.Abi,
		Bytecode:         hex.MustDecodeHex(hexRes.Bytecode),
		DeployedBytecode: hex.MustDecodeHex(hexRes.DeployedBytecode),
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
