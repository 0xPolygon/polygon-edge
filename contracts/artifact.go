package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/umbracle/ethgo/abi"

	"github.com/0xPolygon/polygon-edge/helper/hex"
)

// ReadRawArtifact loads raw SC artifact data from the provided root folder,
// relative path and a file name containing artifact (without extension)
func ReadRawArtifact(rootFolder, contractPath, contractName string) ([]byte, error) {
	fileName := filepath.Join(rootFolder, contractPath, fmt.Sprintf("%s.json", contractName))

	absolutePath, err := filepath.Abs(fileName)
	if err != nil {
		return nil, err
	}

	return os.ReadFile(filepath.Clean(absolutePath))
}

// DecodeArtifact unmarshals provided raw json content into an Artifact instance
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

// LoadArtifactFromFile reads SC artifact file content and decodes it into an Artifact instance
func LoadArtifactFromFile(fileName string) (*Artifact, error) {
	jsonRaw, err := os.ReadFile(filepath.Clean(fileName))
	if err != nil {
		return nil, fmt.Errorf("failed to load artifact from file '%s': %w", fileName, err)
	}

	return DecodeArtifact(jsonRaw)
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
