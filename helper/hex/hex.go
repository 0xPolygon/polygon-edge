package hex

import (
	"fmt"
	"strings"

	"encoding/hex"
)

func EncodeToHex(str []byte) string {
	return "0x" + hex.EncodeToString(str)
}

func EncodeToString(str []byte) string {
	return hex.EncodeToString(str)
}

func DecodeString(str string) ([]byte, error) {
	return hex.DecodeString(str)
}

func DecodeHex(str string) ([]byte, error) {
	if strings.HasPrefix(str, "0x") {
		str = str[2:]
	}
	return hex.DecodeString(str)
}

func MustDecodeHex(str string) []byte {
	buf, err := DecodeHex(str)
	if err != nil {
		panic(fmt.Errorf("could not decode hex: %v", err))
	}
	return buf
}
