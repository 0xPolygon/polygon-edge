package ethash

import (
	"encoding/binary"
	"hash"
	"math/big"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/bitutil"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/sha3"
)

const (
	datasetInitBytes   = 1 << 30 // Bytes in dataset at genesis
	datasetGrowthBytes = 1 << 23 // Dataset growth per epoch
	cacheInitBytes     = 1 << 24 // Bytes in cache at genesis
	cacheGrowthBytes   = 1 << 17 // Cache growth per epoch
	epochLength        = 30000   // Blocks per epoch
	mixBytes           = 128     // Width of mix
	hashBytes          = 64      // Hash length in bytes
	hashWords          = 16      // Number of 32 bit ints in a hash
	datasetParents     = 256     // Number of parents of each dataset element
	cacheRounds        = 3       // Number of rounds in cache production
	loopAccesses       = 64      // Number of accesses in hashimoto loop
)

// cacheSize returns the size of the ethash verification cache that belongs to a certain
// block number.
func cacheSize(block uint64) uint64 {
	epoch := int(block / epochLength)
	if epoch < maxEpoch {
		return cacheSizes[epoch]
	}
	return calcCacheSize(epoch)
}

// calcCacheSize calculates the cache size for epoch. The cache size grows linearly,
// however, we always take the highest prime below the linearly growing threshold in order
// to reduce the risk of accidental regularities leading to cyclic behavior.
func calcCacheSize(epoch int) uint64 {
	size := cacheInitBytes + cacheGrowthBytes*uint64(epoch) - hashBytes
	for !new(big.Int).SetUint64(size / hashBytes).ProbablyPrime(1) { // Always accurate for n < 2^64
		size -= 2 * hashBytes
	}
	return size
}

// datasetSize returns the size of the ethash mining dataset that belongs to a certain
// block number.
func datasetSize(block uint64) uint64 {
	epoch := int(block / epochLength)
	if epoch < maxEpoch {
		return datasetSizes[epoch]
	}
	return calcDatasetSize(epoch)
}

// calcDatasetSize calculates the dataset size for epoch. The dataset size grows linearly,
// however, we always take the highest prime below the linearly growing threshold in order
// to reduce the risk of accidental regularities leading to cyclic behavior.
func calcDatasetSize(epoch int) uint64 {
	size := datasetInitBytes + datasetGrowthBytes*uint64(epoch) - mixBytes
	for !new(big.Int).SetUint64(size / mixBytes).ProbablyPrime(1) { // Always accurate for n < 2^64
		size -= 2 * mixBytes
	}
	return size
}

// hasher is a repetitive hasher allowing the same hash data structures to be
// reused between hash runs instead of requiring new ones to be created.
type hasher func(dest []byte, data []byte)

// makeHasher creates a repetitive hasher, allowing the same hash data structures to
// be reused between hash runs instead of requiring new ones to be created. The returned
// function is not thread safe!
func makeHasher(h hash.Hash) hasher {
	// sha3.state supports Read to get the sum, use it to avoid the overhead of Sum.
	// Read alters the state but we reset the hash before every operation.
	type readerHash interface {
		hash.Hash
		Read([]byte) (int, error)
	}
	rh, ok := h.(readerHash)
	if !ok {
		panic("can't find Read method on hash")
	}
	outputLen := rh.Size()
	return func(dest []byte, data []byte) {
		rh.Reset()
		rh.Write(data)
		rh.Read(dest[:outputLen])
	}
}

// seedHash is the seed to use for generating a verification cache and the mining
// dataset.
func seedHash(block uint64) []byte {
	seed := make([]byte, 32)
	if block < epochLength {
		return seed
	}
	keccak256 := makeHasher(sha3.NewLegacyKeccak256())
	for i := 0; i < int(block/epochLength); i++ {
		keccak256(seed, seed)
	}
	return seed
}

// generateCache creates a verification cache of a given size for an input seed.
// The cache production process involves first sequentially filling up 32 MB of
// memory, then performing two passes of Sergio Demian Lerner's RandMemoHash
// algorithm from Strict Memory Hard Hashing Functions (2014). The output is a
// set of 524288 64-byte values.
// This method places the result into dest in machine byte order.
func generateCache(dest []uint32, epoch uint64, seed []byte) {
	// Print some debug logs to allow analysis on low end devices
	// logger := log.New("epoch", epoch)

	/*
		start := time.Now()
		defer func() {
			elapsed := time.Since(start)

				logFn := logger.Debug
				if elapsed > 3*time.Second {
					logFn = logger.Info
				}
				logFn("Generated ethash verification cache", "elapsed", common.PrettyDuration(elapsed))
		}()
	*/

	// Convert our destination slice to a byte buffer
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&dest))
	header.Len *= 4
	header.Cap *= 4
	cache := *(*[]byte)(unsafe.Pointer(&header))

	// Calculate the number of theoretical rows (we'll store in one buffer nonetheless)
	size := uint64(len(cache))
	rows := int(size) / hashBytes

	// Start a monitoring goroutine to report progress on low end devices
	var progress uint32

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(3 * time.Second):
				// logger.Info("Generating ethash verification cache", "percentage", atomic.LoadUint32(&progress)*100/uint32(rows)/4, "elapsed", common.PrettyDuration(time.Since(start)))
			}
		}
	}()
	// Create a hasher to reuse between invocations
	keccak512 := makeHasher(sha3.NewLegacyKeccak512())

	// Sequentially produce the initial dataset
	keccak512(cache, seed)
	for offset := uint64(hashBytes); offset < size; offset += hashBytes {
		keccak512(cache[offset:], cache[offset-hashBytes:offset])
		atomic.AddUint32(&progress, 1)
	}
	// Use a low-round version of randmemohash
	temp := make([]byte, hashBytes)

	for i := 0; i < cacheRounds; i++ {
		for j := 0; j < rows; j++ {
			var (
				srcOff = ((j - 1 + rows) % rows) * hashBytes
				dstOff = j * hashBytes
				xorOff = (binary.LittleEndian.Uint32(cache[dstOff:]) % uint32(rows)) * hashBytes
			)
			bitutil.XORBytes(temp, cache[srcOff:srcOff+hashBytes], cache[xorOff:xorOff+hashBytes])
			keccak512(cache[dstOff:], temp)

			atomic.AddUint32(&progress, 1)
		}
	}
	// Swap the byte order on big endian systems and return
	if !isLittleEndian() {
		swap(cache)
	}
}

// isLittleEndian returns whether the local system is running in little or big
// endian byte order.
func isLittleEndian() bool {
	n := uint32(0x01020304)
	return *(*byte)(unsafe.Pointer(&n)) == 0x04
}

// swap changes the byte order of the buffer assuming a uint32 representation.
func swap(buffer []byte) {
	for i := 0; i < len(buffer); i += 4 {
		binary.BigEndian.PutUint32(buffer[i:], binary.LittleEndian.Uint32(buffer[i:]))
	}
}

// fnv is an algorithm inspired by the FNV hash, which in some cases is used as
// a non-associative substitute for XOR. Note that we multiply the prime with
// the full 32-bit input, in contrast with the FNV-1 spec which multiplies the
// prime with one byte (octet) in turn.
func fnv(a, b uint32) uint32 {
	return a*0x01000193 ^ b
}

// fnvHash mixes in data into mix using the ethash fnv method.
func fnvHash(mix []uint32, data []uint32) {
	for i := 0; i < len(mix); i++ {
		mix[i] = mix[i]*0x01000193 ^ data[i]
	}
}

// generateDatasetItem combines data from 256 pseudorandomly selected cache nodes,
// and hashes that to compute a single dataset node.
func generateDatasetItem(cache []uint32, index uint32, keccak512 hasher) []byte {
	// Calculate the number of theoretical rows (we use one buffer nonetheless)
	rows := uint32(len(cache) / hashWords)

	// Initialize the mix
	mix := make([]byte, hashBytes)

	binary.LittleEndian.PutUint32(mix, cache[(index%rows)*hashWords]^index)
	for i := 1; i < hashWords; i++ {
		binary.LittleEndian.PutUint32(mix[i*4:], cache[(index%rows)*hashWords+uint32(i)])
	}
	keccak512(mix, mix)

	// Convert the mix to uint32s to avoid constant bit shifting
	intMix := make([]uint32, hashWords)
	for i := 0; i < len(intMix); i++ {
		intMix[i] = binary.LittleEndian.Uint32(mix[i*4:])
	}
	// fnv it with a lot of random cache nodes based on index
	for i := uint32(0); i < datasetParents; i++ {
		parent := fnv(index^i, intMix[i%16]) % rows
		fnvHash(intMix, cache[parent*hashWords:])
	}
	// Flatten the uint32 mix into a binary one and return
	for i, val := range intMix {
		binary.LittleEndian.PutUint32(mix[i*4:], val)
	}
	keccak512(mix, mix)
	return mix
}

// generateDataset generates the entire ethash dataset for mining.
// This method places the result into dest in machine byte order.
func generateDataset(dest []uint32, epoch uint64, cache []uint32) {
	// Print some debug logs to allow analysis on low end devices
	// logger := log.New("epoch", epoch)

	/*
		start := time.Now()
		defer func() {
			elapsed := time.Since(start)

				logFn := logger.Debug
				if elapsed > 3*time.Second {
					logFn = logger.Info
				}
				logFn("Generated ethash verification cache", "elapsed", common.PrettyDuration(elapsed))

		}()
	*/

	// Figure out whether the bytes need to be swapped for the machine
	swapped := !isLittleEndian()

	// Convert our destination slice to a byte buffer
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&dest))
	header.Len *= 4
	header.Cap *= 4
	dataset := *(*[]byte)(unsafe.Pointer(&header))

	// Generate the dataset on many goroutines since it takes a while
	threads := runtime.NumCPU()
	size := uint64(len(dataset))

	var pend sync.WaitGroup
	pend.Add(threads)

	var progress uint32
	for i := 0; i < threads; i++ {
		go func(id int) {
			defer pend.Done()

			// Create a hasher to reuse between invocations
			keccak512 := makeHasher(sha3.NewLegacyKeccak512())

			// Calculate the data segment this thread should generate
			batch := uint32((size + hashBytes*uint64(threads) - 1) / (hashBytes * uint64(threads)))
			first := uint32(id) * batch
			limit := first + batch
			if limit > uint32(size/hashBytes) {
				limit = uint32(size / hashBytes)
			}
			// Calculate the dataset segment
			percent := uint32(size / hashBytes / 100)
			for index := first; index < limit; index++ {
				item := generateDatasetItem(cache, index, keccak512)
				if swapped {
					swap(item)
				}
				copy(dataset[index*hashBytes:], item)

				if status := atomic.AddUint32(&progress, 1); status%percent == 0 {
					// logger.Info("Generating DAG in progress", "percentage", uint64(status*100)/(size/hashBytes), "elapsed", common.PrettyDuration(time.Since(start)))
				}
			}
		}(i)
	}
	// Wait for all the generators to finish and return
	pend.Wait()
}

// hashimoto aggregates data from the full dataset in order to produce our final
// value for a particular header hash and nonce.
func hashimoto(hash []byte, nonce uint64, size uint64, lookup func(index uint32) []uint32) ([]byte, []byte) {
	// Calculate the number of theoretical rows (we use one buffer nonetheless)
	rows := uint32(size / mixBytes)

	// Combine header+nonce into a 64 byte seed
	seed := make([]byte, 40)
	copy(seed, hash)
	binary.LittleEndian.PutUint64(seed[32:], nonce)

	seed = crypto.Keccak512(seed)
	seedHead := binary.LittleEndian.Uint32(seed)

	// Start the mix with replicated seed
	mix := make([]uint32, mixBytes/4)
	for i := 0; i < len(mix); i++ {
		mix[i] = binary.LittleEndian.Uint32(seed[i%16*4:])
	}
	// Mix in random dataset nodes
	temp := make([]uint32, len(mix))

	for i := 0; i < loopAccesses; i++ {
		parent := fnv(uint32(i)^seedHead, mix[i%len(mix)]) % rows
		for j := uint32(0); j < mixBytes/hashBytes; j++ {
			copy(temp[j*hashWords:], lookup(2*parent+j))
		}
		fnvHash(mix, temp)
	}
	// Compress mix
	for i := 0; i < len(mix); i += 4 {
		mix[i/4] = fnv(fnv(fnv(mix[i], mix[i+1]), mix[i+2]), mix[i+3])
	}
	mix = mix[:len(mix)/4]

	digest := make([]byte, common.HashLength)
	for i, val := range mix {
		binary.LittleEndian.PutUint32(digest[i*4:], val)
	}
	return digest, crypto.Keccak256(append(seed, digest...))
}

// hashimotoLight aggregates data from the full dataset (using only a small
// in-memory cache) in order to produce our final value for a particular header
// hash and nonce.
func hashimotoLight(size uint64, cache []uint32, hash []byte, nonce uint64) ([]byte, []byte) {
	keccak512 := makeHasher(sha3.NewLegacyKeccak512())

	lookup := func(index uint32) []uint32 {
		rawData := generateDatasetItem(cache, index, keccak512)

		data := make([]uint32, len(rawData)/4)
		for i := 0; i < len(data); i++ {
			data[i] = binary.LittleEndian.Uint32(rawData[i*4:])
		}
		return data
	}
	return hashimoto(hash, nonce, size, lookup)
}

// hashimotoFull aggregates data from the full dataset (using the full in-memory
// dataset) in order to produce our final value for a particular header hash and
// nonce.
func hashimotoFull(dataset []uint32, hash []byte, nonce uint64) ([]byte, []byte) {
	lookup := func(index uint32) []uint32 {
		offset := index * hashWords
		return dataset[offset : offset+hashWords]
	}
	return hashimoto(hash, nonce, uint64(len(dataset))*4, lookup)
}
