package common

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/sethvargo/go-retry"
)

var (
	// MaxSafeJSInt represents max value which JS support
	// It is used for smartContract fields
	// Our staking repo is written in JS, as are many other clients
	// If we use higher value JS will not be able to parse it
	MaxSafeJSInt = uint64(math.Pow(2, 53) - 2)

	errInvalidDuration = errors.New("invalid duration")
)

// RetryForever will execute a function until it completes without error
func RetryForever(ctx context.Context, interval time.Duration, fn func(context.Context) error) {
	_ = retry.Do(ctx, retry.NewConstant(interval), func(context.Context) error {
		if err := fn(ctx); err != nil {
			return retry.RetryableError(err)
		}

		return nil
	})
}

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

// BigMin returns the smallest of x or y.
func BigMin(x, y *big.Int) *big.Int {
	if x.Cmp(y) > 0 {
		return y
	}

	return x
}

// BigMin returns the smallest of x or y.
func BigMin(x, y *big.Int) *big.Int {
	if x.Cmp(y) > 0 {
		return y
	}

	return x
}

func ConvertUnmarshalledInt(x interface{}) (int64, error) {
	switch tx := x.(type) {
	case float64:
		return roundFloat(tx), nil
	case string:
		v, err := ParseUint64orHex(&tx)
		if err != nil {
			return 0, err
		}

		return int64(v), nil
	default:
		return 0, errors.New("unsupported type for unmarshalled integer")
	}
}

// ParseUint64orHex parses the given uint64 hex string into the number.
// It can parse the string with 0x prefix as well.
func ParseUint64orHex(val *string) (uint64, error) {
	if val == nil {
		return 0, nil
	}

	str := *val
	base := 10

	if strings.HasPrefix(str, "0x") {
		str = str[2:]
		base = 16
	}

	return strconv.ParseUint(str, base, 64)
}

func roundFloat(num float64) int64 {
	return int64(num + math.Copysign(0.5, num))
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

// Duration is a wrapper struct for time.Duration which implements json (un)marshaling
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var (
		v   interface{}
		err error
	)

	if err = json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)

		return nil
	case string:
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}

		return nil
	}

	return errInvalidDuration
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

// EncodeUint64ToBytes encodes provided uint64 to big endian byte slice
func EncodeUint64ToBytes(value uint64) []byte {
	result := make([]byte, 8)
	binary.BigEndian.PutUint64(result, value)

	return result
}

// EncodeBytesToUint64 big endian byte slice to uint64
func EncodeBytesToUint64(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}
