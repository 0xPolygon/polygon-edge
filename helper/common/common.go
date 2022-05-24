package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	// MaxSafeJSInt represents max value which JS support
	// It is used for smartContract fields
	// Our staking repo is written in JS, as are many other clients
	// If we use higher value JS will not be able to parse it
	MaxSafeJSInt = uint64(math.Pow(2, 53) - 2)
)

// Min returns the strictly lower number
func Min(a, b uint64) uint64 {
	if a < b {
		return a
	}

	return b
}

// Max returns the strictly bigger number
func Max(a, b uint64) uint64 {
	if a > b {
		return a
	}

	return b
}

func ConvertUnmarshalledInt(x interface{}) (int64, error) {
	switch tx := x.(type) {
	case float64:
		return roundFloat(tx), nil
	case string:
		v, err := types.ParseUint64orHex(&tx)
		if err != nil {
			return 0, err
		}

		return int64(v), nil
	default:
		return 0, errors.New("unsupported type for unmarshalled integer")
	}
}

func roundFloat(num float64) int64 {
	return int64(num + math.Copysign(0.5, num))
}

func ToFixedFloat(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))

	return float64(roundFloat(num*output)) / output
}

// SetupDataDir sets up the data directory and the corresponding sub-directories
func SetupDataDir(dataDir string, paths []string) error {
	if err := createDir(dataDir); err != nil {
		return fmt.Errorf("failed to create data dir: (%s): %w", dataDir, err)
	}

	for _, path := range paths {
		path := filepath.Join(dataDir, path)
		if err := createDir(path); err != nil {
			return fmt.Errorf("failed to create path: (%s): %w", path, err)
		}
	}

	return nil
}

// DirectoryExists checks if the directory at the specified path exists
func DirectoryExists(directoryPath string) bool {
	// Grab the absolute filepath
	pathAbs, err := filepath.Abs(directoryPath)
	if err != nil {
		return false
	}

	// Check if the directory exists, and that it's actually a directory if there is a hit
	if fileInfo, statErr := os.Stat(pathAbs); os.IsNotExist(statErr) || (fileInfo != nil && !fileInfo.IsDir()) {
		return false
	}

	return true
}

// createDir creates a file system directory if it doesn't exist
func createDir(path string) error {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}

// JSONNumber is the number represented in decimal or hex in json
type JSONNumber struct {
	Value uint64
}

func (d *JSONNumber) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, hex.EncodeUint64(d.Value))), nil
}

func (d *JSONNumber) UnmarshalJSON(data []byte) error {
	var rawValue interface{}
	if err := json.Unmarshal(data, &rawValue); err != nil {
		return err
	}

	val, err := ConvertUnmarshalledInt(rawValue)
	if err != nil {
		return err
	}

	if val < 0 {
		return errors.New("must be positive value")
	}

	d.Value = uint64(val)

	return nil
}

// GetTerminationSignalCh returns a channel to emit signals by ctrl + c
func GetTerminationSignalCh() <-chan os.Signal {
	// wait for the user to quit with ctrl-c
	signalCh := make(chan os.Signal, 1)
	signal.Notify(
		signalCh,
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)

	return signalCh
}

// PadLeftOrTrim left-pads the passed in byte array to the specified size,
// or trims the array if it exceeds the passed in size
func PadLeftOrTrim(bb []byte, size int) []byte {
	l := len(bb)
	if l == size {
		return bb
	}

	if l > size {
		return bb[l-size:]
	}

	tmp := make([]byte, size)
	copy(tmp[size-l:], bb)

	return tmp
}
