package types

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"
	"unicode"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
)

var ZeroAddress = Address{}
var ZeroHash = Hash{}

const (
	HashLength    = 32
	AddressLength = 20
)

const SignatureSize = 4

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

// TODO: Replace jsonrpc/types/argByte with this?
// Still unsure if the codification will be done on protobuf side more
// than marshaling in json and if this will become necessary.
//
//nolint:godox
type ArgBytes []byte

func (b ArgBytes) MarshalText() ([]byte, error) {
	return encodeToHex(b), nil
}

func (b *ArgBytes) UnmarshalText(input []byte) error {
	hh, err := decodeToHex(input)
	if err != nil {
		return nil
	}

	aux := make([]byte, len(hh))
	copy(aux[:], hh[:])
	*b = aux

	return nil
}

func decodeToHex(b []byte) ([]byte, error) {
	str := string(b)
	str = strings.TrimPrefix(str, "0x")

	if len(str)%2 != 0 {
		str = "0" + str
	}

	return hex.DecodeString(str)
}

func encodeToHex(b []byte) []byte {
	str := hex.EncodeToString(b)
	if len(str)%2 != 0 {
		str = "0" + str
	}

	return []byte("0x" + str)
}

var (
	// EmptyRootHash is the root when there are no transactions
	EmptyRootHash = StringToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

	// EmptyUncleHash is the root when there are no uncles
	EmptyUncleHash = StringToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")
)

type Trace struct {
	// AccountTrie is the partial trie for the account merkle trie touched during the block
	AccountTrie map[string]string `json:"accountTrie"`

	// StorageTrie is the partial trie for the storage tries touched during the block
	StorageTrie map[string]string `json:"storageTrie"`

	// ParentStateRoot is the parent state root for this block
	ParentStateRoot Hash `json:"parentStateRoot"`

	// TxnTraces is the list of traces per transaction in the block
	TxnTraces []*TxnTrace `json:"transactionTraces"`
}

type TxnTrace struct {
	// Transaction is the RLP encoding of the transaction
	Transaction ArgBytes `json:"txn"`

	// Delta is the list of updates per account during this transaction
	Delta map[Address]*JournalEntry `json:"delta"`
}

type JournalEntry struct {
	// Addr is the address of the account affected by the
	// journal change
	Addr Address `json:"address"`

	// Balance tracks changes in the account Balance
	Balance *big.Int `json:"balance,omitempty"`

	// Nonce tracks changes in the account Nonce
	Nonce *uint64 `json:"nonce,omitempty"`

	// Storage track changes in the storage
	Storage map[Hash]Hash `json:"storage,omitempty"`

	// StorageRead is the list of storage slots read
	StorageRead map[Hash]struct{} `json:"storage_read,omitempty"`

	// Code tracks the initialization of the contract Code
	Code []byte `json:"code,omitempty"`

	// CodeRead tracks whether the contract Code was read
	CodeRead []byte `json:"code_read,omitempty"`

	// Suicide tracks whether the contract has been self destructed
	Suicide *bool `json:"suicide,omitempty"`

	// Touched tracks whether the account has been touched/created
	Touched *bool `json:"touched,omitempty"`

	// Read signals whether the account was read
	Read *bool `json:"read,omitempty"`
}

func (j *JournalEntry) Merge(jj *JournalEntry) {
	if jj.Nonce != nil && jj.Nonce != j.Nonce {
		j.Nonce = jj.Nonce
	}

	if jj.Balance != nil && jj.Balance != j.Balance {
		j.Balance = jj.Balance
	}

	if jj.Storage != nil {
		if j.Storage == nil {
			j.Storage = map[Hash]Hash{}
		}

		for k, v := range jj.Storage {
			j.Storage[k] = v
		}
	}

	if jj.Code != nil && !bytes.Equal(jj.Code, j.Code) {
		j.Code = jj.Code
	}

	if jj.CodeRead != nil && !bytes.Equal(jj.CodeRead, j.CodeRead) {
		j.CodeRead = jj.CodeRead
	}

	if jj.Suicide != nil && jj.Suicide != j.Suicide {
		j.Suicide = jj.Suicide
	}

	if jj.Touched != nil && jj.Touched != j.Touched {
		j.Touched = jj.Touched
	}

	if jj.Read != nil && jj.Read != j.Read {
		j.Read = jj.Read
	}

	if jj.StorageRead != nil {
		if j.StorageRead == nil {
			j.StorageRead = map[Hash]struct{}{}
		}

		for k := range jj.StorageRead {
			j.StorageRead[k] = struct{}{}
		}
	}
}

type Proof struct {
	Data     []Hash // the proof himself
	Metadata map[string]interface{}
}
