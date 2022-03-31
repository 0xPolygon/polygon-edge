package fastmerkle

import (
	"fmt"
)

// workerPool is the pool of worker threads
// that parse hashing jobs
type workerPool struct {
	resultsCh chan *workerResult // The channel to relay results to
}

// workerJob is a single hashing job performed
// by the worker thread
type workerJob struct {
	storeIndex int      // the final store index after hashing
	sourceData [][]byte // the reference to the two items that need to be hashed
}

// workerResult is the result of the worker thread's hashing job
type workerResult struct {
	storeIndex int    // the final store index after hashing
	hashData   []byte // the actual hash result data
	error      error  // any kind of error that occurred during hashing
}

// newWorkerPool spawns a new worker pool
func newWorkerPool(expectedNumResults int) *workerPool {
	return &workerPool{
		resultsCh: make(chan *workerResult, expectedNumResults),
	}
}

// addJob adds a new job asynchronously to be processed by the worker pool
func (wp *workerPool) addJob(job *workerJob) {
	go wp.runJob(job)
}

// getResult takes out a result from the worker pool [Blocking]
func (wp *workerPool) getResult() *workerResult {
	return <-wp.resultsCh
}

// close closes the worker pool and their corresponding
// channels
func (wp *workerPool) close() {
	close(wp.resultsCh)
}

// runJob is the main activity method for the
// worker threads, there new jobs are parsed and results sent out
func (wp *workerPool) runJob(job *workerJob) {
	// Grab an instance of the fast hasher
	hasher := acquireFastHasher()
	// Release the hasher when it's no longer needed
	defer releaseFastHasher(hasher)

	// Concatenate all items that need to be hashed together
	preparedArray := make([]byte, 0)
	for i := 0; i < len(job.sourceData); i++ {
		preparedArray = append(preparedArray, job.sourceData[i]...)
	}

	// Hash the items in the job
	err := hasher.addToHash(preparedArray)
	if err != nil {
		err = fmt.Errorf(
			"unable to write hash, %w",
			err,
		)
	}

	// Construct a hash result from the fast hasher
	result := &workerResult{
		storeIndex: job.storeIndex,
		hashData:   hasher.getHash(),
		error:      err,
	}

	// Report the result back
	wp.resultsCh <- result
}
