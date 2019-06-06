package ethash

import (
	"encoding/binary"
	"math/big"

	"github.com/umbracle/minimal/types"
	"golang.org/x/crypto/sha3"

	"github.com/umbracle/minimal/rlp"
)

// REVISION is the spec revision number of Ethash
const REVISION = 23

var (
	// bytes in word
	wordBytes = 4
	// bytes in dataset at genesis (2 ** 30)
	datasetBytesInit = 1 << 30
	// dataset growth per epoch (2 ** 23)
	datasetBytesGrowth = 1 << 23
	// bytes in cache at genesis (2 ** 24)
	cacheBytesInit = 1 << 24
	// cache growth per epoch (2 ** 17)
	cacheBytesGrowth = 1 << 17
	// Size of the DAG relative to the cache
	cacheMultiplier = 1024
	// blocks per epoch
	epochLength = 30000
	// width of mix
	mixBytes = 128
	// hash length in bytes
	hashBytes = 64
	// number of parents of each dataset element
	datasetParents = 256
	// number of rounds in cache production
	cacheRounds = 3
	// number of accesses in hashimoto loop
	accesses = 64
)

func fnvOp(v1, v2 uint32) uint32 {
	return (v1 * 0x01000193) ^ v2
}

func getSeedHashByEpoch(epoch int) []byte {
	seed := make([]byte, 32)
	if epoch == 0 {
		return seed
	}
	h := sha3.NewLegacyKeccak256()
	for i := 0; i < epoch; i++ {
		h.Write(seed)
		seed = h.Sum(nil)
		h.Reset()
	}
	return seed
}

func getCacheSizeByEpoch(epoch int) uint64 {
	if epoch < maxEpoch {
		return cacheSizes[epoch]
	}
	return calcCacheSizeByEpoch(epoch)
}

func getDatasetSizeByEpoch(epoch int) uint64 {
	if epoch < maxEpoch {
		return datasetSizes[epoch]
	}
	return calcDatasetSizeByEpoch(epoch)
}

func calcSizeByEpoch(epoch, init, growth, mix int) uint64 {
	sz := init + growth*(epoch)
	sz -= mix
	aux := big.NewInt(0)

BACK:
	aux.SetInt64(int64(sz / mix))
	if !aux.ProbablyPrime(1) {
		sz -= 2 * mix
		goto BACK
	}
	return uint64(sz)
}

func calcCacheSizeByEpoch(epoch int) uint64 {
	return calcSizeByEpoch(epoch, cacheBytesInit, cacheBytesGrowth, hashBytes)
}

func calcDatasetSizeByEpoch(epoch int) uint64 {
	return calcSizeByEpoch(epoch, datasetBytesInit, datasetBytesGrowth, mixBytes)
}

func getSeedHash(num int) []byte {
	return getSeedHashByEpoch(num / epochLength)
}

type lookupFn func(index uint32) []uint32

type hashFn func(p []byte) []byte

func hashimoto(header []byte, nonce uint64, fullSize int, sha512, sha256 hashFn, lookup lookupFn) ([]byte, []byte) {
	w := mixBytes / wordBytes
	n := uint32(fullSize / mixBytes)

	// combine header+nonce into a 64 byte seed
	s := make([]byte, 40)
	copy(s, header)
	binary.LittleEndian.PutUint64(s[32:], nonce)

	s = sha512(s)
	sHead := binary.LittleEndian.Uint32(s)

	// start the mix with replicated s
	mix := make([]uint32, w)
	for i := 0; i < len(mix); i++ {
		mix[i] = binary.LittleEndian.Uint32(s[i%16*4:])
	}

	tmp := make([]uint32, w)

	// mix in random dataset nodes
	for i := 0; i < accesses; i++ {
		p := fnvOp(uint32(i)^sHead, mix[i%w]) % n

		// NOTE: mixBytes/hashBytes = 2. Would it be faster to unroll this loop?
		for j := 0; j < mixBytes/hashBytes; j++ {
			copy(tmp[j*16:], lookup(2*p+uint32(j)))
		}

		// fnv map
		for o := 0; o < 32; o++ {
			mix[o] = fnvOp(mix[o], tmp[o])
		}
	}

	// compress mix
	cmix := []uint32{}
	for i := 0; i < len(mix); i += 4 {
		cmix = append(cmix, fnvOp(fnvOp(fnvOp(mix[i], mix[i+1]), mix[i+2]), mix[i+3]))
	}

	digest := make([]byte, 32)
	for i, val := range cmix {
		binary.LittleEndian.PutUint32(digest[i*4:], val)
	}

	result := sha256(append(s, digest...))
	return digest, result
}

func sealHash(header *types.Header) (hash types.Hash) {
	hasher := sha3.NewLegacyKeccak256()

	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.Sha3Uncles,
		header.Miner,
		header.StateRoot,
		header.TxRoot,
		header.ReceiptsRoot,
		header.LogsBloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Timestamp,
		header.ExtraData,
	})
	hasher.Sum(hash[:0])
	return hash
}
