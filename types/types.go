package types

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
)

const (
	HashLength    = 32
	AddressLength = 20

	SignatureSize = 4
)

var (
	// ZeroAddress is the default zero address
	ZeroAddress = Address{}

	// ZeroHash is the default zero hash
	ZeroHash = Hash{}

	// ZeroNonce is the default empty nonce
	ZeroNonce = Nonce{}

	// EmptyRootHash is the root when there are no transactions
	EmptyRootHash = StringToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

	// EmptyUncleHash is the root when there are no uncles
	EmptyUncleHash = StringToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")

	// EmptyCodeHash is the root where there is no code.
	// Equivalent of: `types.BytesToHash(crypto.Keccak256(nil))`
	EmptyCodeHash = StringToHash("0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470")

	// ErrTxTypeNotSupported denotes that transaction is not supported
	ErrTxTypeNotSupported = errors.New("transaction type not supported")

	// ErrInsufficientFunds denotes that account has insufficient funds for transaction execution
	ErrInsufficientFunds = errors.New("insufficient funds for execution")
)

type Hash [HashLength]byte

type Address [AddressLength]byte

func min(i, j int) int {
	if i < j {
		return i
	}

	return j
}

func BytesToHash(b []byte) Hash {
	var h Hash

	size := len(b)
	min := min(size, HashLength)

	copy(h[HashLength-min:], b[len(b)-min:])

	return h
}

func (h Hash) Bytes() []byte {
	return h[:]
}

func (h Hash) String() string {
	return hex.EncodeToHex(h[:])
}

// checksumEncode returns the checksummed address with 0x prefix, as by EIP-55
// https://github.com/ethereum/EIPs/blob/master/EIPS/eip-55.md
func (a Address) checksumEncode() string {
	addrBytes := a.Bytes() // 20 bytes

	// Encode to hex without the 0x prefix
	lowercaseHex := hex.EncodeToHex(addrBytes)[2:]
	hashedAddress := hex.EncodeToHex(keccak.Keccak256(nil, []byte(lowercaseHex)))[2:]

	result := make([]rune, len(lowercaseHex))
	// Iterate over each character in the lowercase hex address
	for idx, ch := range lowercaseHex {
		if ch >= '0' && ch <= '9' || hashedAddress[idx] >= '0' && hashedAddress[idx] <= '7' {
			// Numbers in range [0, 9] are ignored (as well as hashed values [0, 7]),
			// because they can't be uppercased
			result[idx] = ch
		} else {
			// The current character / hashed character is in the range [8, f]
			result[idx] = unicode.ToUpper(ch)
		}
	}

	return "0x" + string(result)
}

func (a Address) Ptr() *Address {
	return &a
}

func (a Address) String() string {
	return a.checksumEncode()
}

func (a Address) Bytes() []byte {
	return a[:]
}

func StringToHash(str string) Hash {
	return BytesToHash(StringToBytes(str))
}

func StringToAddress(str string) Address {
	return BytesToAddress(StringToBytes(str))
}

func AddressToString(address Address) string {
	return string(address[:])
}

func BytesToAddress(b []byte) Address {
	var a Address

	size := len(b)
	min := min(size, AddressLength)

	copy(a[AddressLength-min:], b[len(b)-min:])

	return a
}

func StringToBytes(str string) []byte {
	str = strings.TrimPrefix(str, "0x")
	if len(str)%2 == 1 {
		str = "0" + str
	}

	b, _ := hex.DecodeString(str)

	return b
}

// IsValidAddress checks if provided string is a valid Ethereum address
func IsValidAddress(address string) error {
	// remove 0x prefix if it exists
	if strings.HasPrefix(address, "0x") {
		address = address[2:]
	}

	// decode the address
	decodedAddress, err := hex.DecodeString(address)
	if err != nil {
		return fmt.Errorf("address %s contains invalid characters", address)
	}

	// check if the address has the correct length
	if len(decodedAddress) != AddressLength {
		return fmt.Errorf("address %s has invalid length", string(decodedAddress))
	}

	return nil
}

// UnmarshalText parses a hash in hex syntax.
func (h *Hash) UnmarshalText(input []byte) error {
	*h = BytesToHash(StringToBytes(string(input)))

	return nil
}

// UnmarshalText parses an address in hex syntax.
func (a *Address) UnmarshalText(input []byte) error {
	buf := StringToBytes(string(input))
	if len(buf) != AddressLength {
		return fmt.Errorf("incorrect length")
	}

	*a = BytesToAddress(buf)

	return nil
}

func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

func (a Address) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

type Proof struct {
	Data     []Hash // the proof himself
	Metadata map[string]interface{}
}

type OverrideAccount struct {
	Nonce     *uint64
	Code      []byte
	Balance   *big.Int
	State     map[Hash]Hash
	StateDiff map[Hash]Hash
}

type StateOverride map[Address]OverrideAccount
