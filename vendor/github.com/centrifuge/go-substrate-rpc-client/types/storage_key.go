// Go Substrate RPC Client (GSRPC) provides APIs and types around Polkadot and any Substrate-based chain RPC calls
//
// Copyright 2019 Centrifuge GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	"fmt"
	"io"

	"github.com/centrifuge/go-substrate-rpc-client/scale"
	"github.com/centrifuge/go-substrate-rpc-client/xxhash"
)

// StorageKey represents typically hashed storage keys of the system.
// Be careful using this in your own structs – it only works as the last value in a struct since it will consume the
// remainder of the encoded data. The reason for this is that it does not contain any length encoding, so it would
// not know where to stop.
type StorageKey []byte

// NewStorageKey creates a new StorageKey type
func NewStorageKey(b []byte) StorageKey {
	return b
}

// CreateStorageKey uses the given metadata and to derive the right hashing of method, prefix as well as arguments to
// create a hashed StorageKey
func CreateStorageKey(meta *Metadata, prefix, method string, arg []byte, arg2 []byte) (StorageKey, error) {
	stringKey := []byte(prefix + " " + method)

	entryMeta, err := meta.FindStorageEntryMetadata(prefix, method)
	if err != nil {
		return nil, err
	}

	if entryMeta.IsDoubleMap() {
		return createKeyDoubleMap(meta, method, prefix, stringKey, arg, arg2, entryMeta)
	}

	return createKey(meta, method, prefix, stringKey, arg, entryMeta)
}

// Encode implements encoding for StorageKey, which just unwraps the bytes of StorageKey
func (s StorageKey) Encode(encoder scale.Encoder) error {
	return encoder.Write(s)
}

// Decode implements decoding for StorageKey, which just reads all the remaining bytes into StorageKey
func (s *StorageKey) Decode(decoder scale.Decoder) error {
	for i := 0; true; i++ {
		b, err := decoder.ReadOneByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		*s = append((*s)[:i], b)
	}
	return nil
}

// Hex returns a hex string representation of the value (not of the encoded value)
func (s StorageKey) Hex() string {
	return fmt.Sprintf("%#x", s)
}

// createKeyDoubleMap creates a key for a DoubleMap type
func createKeyDoubleMap(meta *Metadata, method, prefix string, stringKey, arg, arg2 []byte,
	entryMeta StorageEntryMetadata) (StorageKey, error) {
	if arg == nil || arg2 == nil {
		return nil, fmt.Errorf("%v is a DoubleMap and requires two arguments", method)
	}

	hasher, err := entryMeta.Hasher()
	if err != nil {
		return nil, err
	}

	hasher2, err := entryMeta.Hasher2()
	if err != nil {
		return nil, err
	}

	if meta.Version <= 8 {
		_, err := hasher.Write(append(stringKey, arg...))
		if err != nil {
			return nil, err
		}
		_, err = hasher2.Write(arg2)
		if err != nil {
			return nil, err
		}
		return append(hasher.Sum(nil), hasher2.Sum(nil)...), err
	}

	_, err = hasher.Write(arg)
	if err != nil {
		return nil, err
	}
	_, err = hasher2.Write(arg2)
	if err != nil {
		return nil, err
	}

	key := createPrefixedKey(method, prefix)
	key = append(key, hasher.Sum(nil)...)
	key = append(key, hasher2.Sum(nil)...)

	return key, nil
}

// createKey creates a key for either a map or a plain value
func createKey(meta *Metadata, method, prefix string, stringKey, arg []byte, entryMeta StorageEntryMetadata) (
	StorageKey, error) {
	if entryMeta.IsMap() && arg == nil {
		return nil, fmt.Errorf("%v is a Map and requires one argument", method)
	}

	hasher, err := entryMeta.Hasher()
	if err != nil {
		return nil, err
	}

	if meta.Version <= 8 {
		_, err := hasher.Write(append(stringKey, arg...))
		return hasher.Sum(nil), err
	}

	if entryMeta.IsMap() {
		_, err := hasher.Write(arg)
		if err != nil {
			return nil, err
		}
		arg = hasher.Sum(nil)
	}

	return append(createPrefixedKey(method, prefix), arg...), nil
}

func createPrefixedKey(method, prefix string) []byte {
	return append(xxhash.New128([]byte(prefix)).Sum(nil), xxhash.New128([]byte(method)).Sum(nil)...)
}
