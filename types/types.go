package types

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"

	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
)

var ZeroAddress = Address{}
var ZeroHash = Hash{}

const (
	HashLength    = 32
	AddressLength = 20
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

func (h Hash) Value() (driver.Value, error) {
	return h.String(), nil
}

func (h *Hash) Scan(src interface{}) error {
	hh := hex.MustDecodeHex(string(src.([]byte)))
	copy(h[:], hh[:])
	return nil
}

// EIP55 returns the checksummed address with 0x prefix
// https://github.com/ethereum/EIPs/blob/master/EIPS/eip-55.md
func (a Address) EIP55() string {
	addrBytes := a.Bytes() // 20 bytes

	// Encode to hex
	lowercaseHex := hex.EncodeToHex(addrBytes)[2:] // without the 0x prefix

	hashedAddress := hex.EncodeToHex(keccak.Keccak256(nil, []byte(lowercaseHex)))[2:]

	checksummedBuffer := ""
	// Iterate over each character in the hashed address
	for index, character := range lowercaseHex {
		if character >= '0' && character <= '9' {
			// Numbers in range [0, 9] are ignored,
			// because they can't be uppercased
			checksummedBuffer += string(character)
		} else {
			// Look through range {a, b, c, d, e, f}

			// Check if the corresponding hex digit in the hash is 8 or higher
			hashedNibble, _ := strconv.ParseInt(string(hashedAddress[index]), 16, 64)
			if hashedNibble > 7 {
				checksummedBuffer += strings.ToUpper(string(character))
			} else {
				checksummedBuffer += string(character)
			}
		}
	}

	return "0x" + checksummedBuffer
}

func (a Address) String() string {
	return a.EIP55()
}

func (a Address) Bytes() []byte {
	return a[:]
}

func (a Address) Value() (driver.Value, error) {
	return a.String(), nil
}

func (a *Address) Scan(src interface{}) error {
	aa := hex.MustDecodeHex(string(src.([]byte)))
	copy(a[:], aa[:])
	return nil
}

func StringToHash(str string) Hash {
	return BytesToHash(stringToBytes(str))
}

func StringToAddress(str string) Address {
	return BytesToAddress(stringToBytes(str))
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

func stringToBytes(str string) []byte {
	str = strings.TrimPrefix(str, "0x")
	if len(str)%2 == 1 {
		str = "0" + str
	}
	b, _ := hex.DecodeString(str)
	return b
}

// UnmarshalText parses a hash in hex syntax.
func (h *Hash) UnmarshalText(input []byte) error {
	*h = BytesToHash(stringToBytes(string(input)))
	return nil
}

// UnmarshalText parses an address in hex syntax.
func (a *Address) UnmarshalText(input []byte) error {
	buf := stringToBytes(string(input))
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

var (
	// EmptyRootHash is the root when there are no transactions
	EmptyRootHash = StringToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

	// EmptyUncleHash is the root when there are no uncles
	EmptyUncleHash = StringToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")
)
