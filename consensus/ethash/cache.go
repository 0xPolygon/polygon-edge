package ethash

import (
	"encoding/binary"
	"fmt"
	"hash"

	"golang.org/x/crypto/sha3"
)

// Cache is a 16 MB pseudorandom cache.
type Cache struct {
	cacheSize   uint32
	datasetSize int
	cache       []uint32
	sha512      hash.Hash
	sha256      hash.Hash
	mix         [16]uint32
}

func newCache(epoch int) *Cache {
	cacheSize := getCacheSizeByEpoch(epoch)
	datasetSize := getDatasetSizeByEpoch(epoch)
	seed := getSeedHashByEpoch(epoch)

	c := &Cache{
		sha512:      sha3.NewLegacyKeccak512(),
		sha256:      sha3.NewLegacyKeccak256(),
		datasetSize: int(datasetSize),
	}

	c.mkcache(int(cacheSize), seed)
	c.cacheSize = uint32(len(c.cache))
	return c
}

func (c *Cache) calcDatasetItem(i uint32) []uint32 {
	n := c.cacheSize
	m := uint32(len(c.cache) / 16)
	r := hashBytes / wordBytes

	copy(c.mix[:], c.cache[(i*16)%n:])
	c.mix[0] ^= i
	c.sha512Int(c.mix[:])

	for j := 0; j < datasetParents; j++ {
		cacheIndex := (fnvOp(i^uint32(j), c.mix[j%r]) % m) * 16

		aux := c.cache[cacheIndex : cacheIndex+16]
		for o := 0; o < 16; o++ {
			c.mix[o] = fnvOp(c.mix[o], aux[o])
		}
	}

	c.sha512Int(c.mix[:])
	return c.mix[:]
}

func (c *Cache) sha512Aux(p []byte) []byte {
	c.sha512.Reset()
	c.sha512.Write(p)
	return c.sha512.Sum(nil)
}

func (c *Cache) sha256Aux(p []byte) []byte {
	c.sha256.Reset()
	c.sha256.Write(p)
	return c.sha256.Sum(nil)
}

func (c *Cache) sha512Int(p []uint32) {
	aux := make([]byte, 4)

	c.sha512.Reset()
	for _, i := range p {
		binary.LittleEndian.PutUint32(aux, i)
		c.sha512.Write(aux)
	}
	res := c.sha512.Sum(nil)
	for i := 0; i < len(p); i++ {
		p[i] = binary.LittleEndian.Uint32(res[i*4:])
	}
}

func (c *Cache) mkcache(cacheSize int, seed []byte) {
	n := cacheSize / hashBytes

	res := [][]byte{}
	res = append(res, c.sha512Aux(seed))
	for i := 1; i < n; i++ {
		aux := c.sha512Aux(res[i-1])
		res = append(res, aux)
	}

	for j := 0; j < cacheRounds; j++ {
		for i := 0; i < n; i++ {
			v := binary.LittleEndian.Uint32(res[i]) % uint32(n)
			temp := xorBytes(res[(i-1+n)%n], res[v])
			res[i] = c.sha512Aux(temp)
		}
	}

	// Convert bytes to words
	resInt := []uint32{}
	for _, i := range res {
		entry := make([]uint32, 16)
		for indx := range entry {
			entry[indx] = binary.LittleEndian.Uint32(i[indx*4:])
		}
		resInt = append(resInt, entry...)
	}

	c.cache = resInt
}

func (c *Cache) hashimoto(header []byte, nonce uint64) ([]byte, []byte) {
	return hashimoto(header, nonce, c.datasetSize, c.sha512Aux, c.sha256Aux, c.calcDatasetItem)
}

func xorBytes(a, b []byte) []byte {
	if len(a) != len(b) {
		panic(fmt.Sprintf("length of byte slices is not equivalent: %d != %d", len(a), len(b)))
	}
	buf := make([]byte, len(a))
	for i := range a {
		buf[i] = a[i] ^ b[i]
	}
	return buf
}
