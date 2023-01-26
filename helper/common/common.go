package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/big"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
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
func SetupDataDir(dataDir string, paths []string, perms fs.FileMode) error {
	if err := CreateDirSafe(dataDir, perms); err != nil {
		return fmt.Errorf("failed to create data dir: (%s): %w", dataDir, err)
	}

	for _, path := range paths {
		path := filepath.Join(dataDir, path)
		if err := CreateDirSafe(path, perms); err != nil {
			return fmt.Errorf("failed to create path: (%s): %w", path, err)
		}
	}

	return nil
}

// DirectoryExists checks if the directory at the specified path exists
func DirectoryExists(directoryPath string) bool {
	// Check if path is empty
	if directoryPath == "" {
		return false
	}

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

// Checks if the file at the specified path exists
func FileExists(filePath string) bool {
	// Check if path is empty
	if filePath == "" {
		return false
	}

	// Grab the absolute filepath
	pathAbs, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}

	// Check if the file exists, and that it's actually a file if there is a hit
	if fileInfo, statErr := os.Stat(pathAbs); os.IsNotExist(statErr) || (fileInfo != nil && fileInfo.IsDir()) {
		return false
	}

	return true
}

// Creates a directory at path and with perms level permissions.
// If directory already exists, owner and permissions are verified.
func CreateDirSafe(path string, perms fs.FileMode) error {
	info, err := os.Stat(path)
	// check if an error occurred other than path not exists
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// create directory if it does not exist
	if !DirectoryExists(path) {
		if err := os.MkdirAll(path, perms); err != nil {
			return err
		}

		return nil
	}

	// verify that existing directory's owner and permissions are safe
	err = verifyFileAndTryUpdatePermissions(path, info, perms)
	if err != nil {
		return err
	}

	return nil
}

// Creates a file at path and with perms level permissions.
// If file already exists, owner and permissions are verified.
// If shouldNotExist is true, an error is returned if file exists
func CreateFileSafe(path string, data []byte, perms fs.FileMode, shouldNotExist bool) error {
	info, err := os.Stat(path)
	// check if an error occurred other than path not exists
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	fileExists := FileExists(path)
	if fileExists && shouldNotExist {
		return fmt.Errorf("%s already initialized", path)
	}

	// create file if it does not exist
	if !fileExists {
		if err := os.WriteFile(path, data, perms); err != nil {
			return err
		}

		return nil
	}

	// verify that existing file's owner and permissions are safe
	err = verifyFileAndTryUpdatePermissions(path, info, perms)
	if err != nil {
		return err
	}

	return nil
}

// Verifies the file using `verifyFileOwnerAndPermissions` function
// and updates permissions if current user is the owner
func verifyFileAndTryUpdatePermissions(path string, info fs.FileInfo, expectedPerms fs.FileMode) error {
	err, isCurrUserOwner := verifyFileOwnerAndPermissions(path, info, expectedPerms)
	if err != nil {
		return err
	}

	// update permissions (just in case) if current user is the owner of the directory
	if isCurrUserOwner {
		err := os.Chmod(path, expectedPerms)
		if err != nil {
			return err
		}
	}

	return nil
}

// Verifies that the file owner is the current user, or the file owner is in
// the same group as current user and permissions are set correctly by the owner.
// Returns an error if occurred, and a bool indicating whether the current user is the owner.
func verifyFileOwnerAndPermissions(path string, info fs.FileInfo, expectedPerms fs.FileMode) (error, bool) {
	// get stats
	stat, ok := info.Sys().(*syscall.Stat_t)
	if stat == nil || !ok {
		return fmt.Errorf("failed to get stats of %s", path), false
	}

	// get current user
	currUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user"), false
	}

	// get user id of the owner
	ownerUID := strconv.FormatUint(uint64(stat.Uid), 10)
	if currUser.Uid == ownerUID {
		return nil, true
	}

	// get group id of the owner
	ownerGID := strconv.FormatUint(uint64(stat.Gid), 10)
	if currUser.Gid != ownerGID {
		return fmt.Errorf("file/directory created by a user from a different group: %s", path), false
	}

	// check if permissions are set correctly by the owner
	if info.Mode() != expectedPerms {
		return fmt.Errorf("permissions of the file/directory is set incorrectly by another user: %s", path), false
	}

	return nil, false
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

// ExtendByteSlice extends given byte slice by needLength parameter and trims it
func ExtendByteSlice(b []byte, needLength int) []byte {
	b = b[:cap(b)]

	if n := needLength - len(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}

	return b[:needLength]
}

// BigIntDivCeil performs integer division and rounds given result to next bigger integer number
// It is calculated using this formula result = (a + b - 1) / b
func BigIntDivCeil(a, b *big.Int) *big.Int {
	result := new(big.Int)

	return result.Add(a, b).
		Sub(result, big.NewInt(1)).
		Div(result, b)
}
