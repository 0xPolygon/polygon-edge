package fastmerkle

import (
	"golang.org/x/crypto/sha3"
	"hash"
	"sync"
)

// fastHasher is a hashing instance shared by multiple processes
type fastHasher struct {
	hashEngine hash.Hash // The hash engine that will be used (for now it's Keccak256)
}

// addToHash adds the input data to the hashing engine
func (fh *fastHasher) addToHash(inputData []byte) error {
	_, writeErr := fh.hashEngine.Write(inputData)

	return writeErr
}

// getHash returns the hash of the input data
func (fh *fastHasher) getHash() []byte {
	return fh.hashEngine.Sum(nil)
}

// fastHasherPool is a sync pool that can dish out
// fast hasher instances
var fastHasherPool = sync.Pool{
	New: func() interface{} {
		return &fastHasher{
			hashEngine: sha3.NewLegacyKeccak256(),
		}
	},
}

// acquireFastHasher acquires a new instance of the hasher
// from the hasher pool. Must be coupled with an adequate release call
func acquireFastHasher() *fastHasher {
	fh, _ := fastHasherPool.Get().(*fastHasher)

	return fh
}

// releaseFastHasher resets a hasher instance and puts it back
// into the hasher pool for later usage
func releaseFastHasher(fh *fastHasher) {
	fh.hashEngine.Reset()

	fastHasherPool.Put(fh)
}
