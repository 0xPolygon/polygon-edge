package artifact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/umbracle/ethgo/abi"
)

func ReadArtifactData(rootFolder, contractPath, contractName string) ([]byte, error) {
	fileName := filepath.Join(rootFolder, contractPath, fmt.Sprintf("%s.json", contractName))

	absolutePath, err := filepath.Abs(fileName)
	if err != nil {
		return nil, err
	}

	return os.ReadFile(filepath.Clean(absolutePath))
}

func DecodeArtifact(data []byte) (*Artifact, error) {
	var hexRes HexArtifact
	if err := json.Unmarshal(data, &hexRes); err != nil {
		return nil, fmt.Errorf("artifact found but no correct format: %w", err)
	}

	return &Artifact{
		Abi:              hexRes.Abi,
		Bytecode:         hex.MustDecodeHex(hexRes.ByteCode),
		DeployedBytecode: hex.MustDecodeHex(hexRes.DeployedBytecode),
	}, nil
}

type HexArtifact struct {
	Abi              *abi.ABI
	ByteCode         string
	DeployedBytecode string
}

type Artifact struct {
	Abi              *abi.ABI
	Bytecode         []byte
	DeployedBytecode []byte
}
