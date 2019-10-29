package ethash

import (
	"encoding/binary"
	"math/big"

	"github.com/umbracle/fastrlp"
	"github.com/umbracle/minimal/helper/keccak"
	"github.com/umbracle/minimal/types"
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
	h := keccak.DefaultKeccakPool.Get()
	for i := 0; i < epoch; i++ {
		h.Write(seed)
		seed = h.Sum(seed[:0])
		h.Reset()
	}
	keccak.DefaultKeccakPool.Put(h)
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

var sealArenaPool fastrlp.ArenaPool

func (e *Ethash) sealHash(h *types.Header) []byte {
	arena := sealArenaPool.Get()

	vv := arena.NewArray()
	vv.Set(arena.NewBytes(h.ParentHash.Bytes()))
	vv.Set(arena.NewBytes(h.Sha3Uncles.Bytes()))
	vv.Set(arena.NewBytes(h.Miner.Bytes()))
	vv.Set(arena.NewBytes(h.StateRoot.Bytes()))
	vv.Set(arena.NewBytes(h.TxRoot.Bytes()))
	vv.Set(arena.NewBytes(h.ReceiptsRoot.Bytes()))
	vv.Set(arena.NewBytes(h.LogsBloom[:]))
	vv.Set(arena.NewUint(h.Difficulty))
	vv.Set(arena.NewUint(h.Number))
	vv.Set(arena.NewUint(h.GasLimit))
	vv.Set(arena.NewUint(h.GasUsed))
	vv.Set(arena.NewUint(h.Timestamp))
	vv.Set(arena.NewCopyBytes(h.ExtraData))

	//e.tmp = arena.HashTo(e.tmp[:0], vv)

	e.tmp = e.keccak256.WriteRlp(e.tmp[:0], vv)
	e.keccak256.Reset()

	sealArenaPool.Put(arena)
	return e.tmp
}
