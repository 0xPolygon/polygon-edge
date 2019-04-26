package ethereum

import (
	"testing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/blockchain"
	"github.com/stretchr/testify/assert"
)

func newTestQueue(head *types.Header, to uint64) *queue {
	q := newQueue()
	q.front = q.newItem(head.Number.Uint64() + 1)
	q.head = head.Hash()
	q.addBack(to)
	return q
}

func findElement(t *testing.T, q *queue, id uint32) *element {
	elem, err := q.findElement(id)
	if err != nil {
		t.Fatal(err)
	}
	return elem
}

func dequeue(t *testing.T, q *queue) *Job {
	job, err := q.Dequeue()
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestQueueDeliverPartialHeaders(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 100)

	job := dequeue(t, q)
	assert.Equal(t, job.payload, &HeadersJob{1, 99})

	elem := findElement(t, q, job.id)
	assert.Equal(t, elem.headersStatus, pendingX)

	// Deliver partial number of headers
	assert.NoError(t, q.deliverHeaders(job.id, headers[1:90]))
	assert.Equal(t, elem.headersStatus, waitingX)

	job = dequeue(t, q)
	assert.Equal(t, job.payload, &HeadersJob{90, 10})

	// Deliver wrong number of headers fails
	assert.Error(t, q.deliverHeaders(job.id, headers[91:101]))

	// Deliver the correct number
	assert.NoError(t, q.deliverHeaders(job.id, headers[90:100]))

	// The element is completed
	assert.Equal(t, elem.headersStatus, completedX)
	assert.True(t, elem.Completed())
}

func TestQueueReceiptsAndBodiesCompletedByDefault(t *testing.T) {
	// if we are pending a headers job, the next job to dequeue has to be
	// for another headersjob

	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 1000)

	job1 := dequeue(t, q)
	assert.Equal(t, job1.payload, &HeadersJob{1, 190})

	elem1 := findElement(t, q, job1.id)

	assert.Equal(t, elem1.receiptsStatus, completedX)
	assert.Equal(t, elem1.bodiesStatus, completedX)
	assert.Equal(t, elem1.headersStatus, pendingX)

	job2 := dequeue(t, q)
	assert.Equal(t, job2.payload, &HeadersJob{191, 190})
	
	elem2 := findElement(t, q, job2.id)

	// Deliver job 2
	assert.NoError(t, q.deliverHeaders(job2.id, headers[191:381]))
	// Deliver job 1
	assert.NoError(t, q.deliverHeaders(job1.id, headers[1:191]))

	assert.True(t, elem1.Completed())
	assert.True(t, elem2.Completed())
}

func TestQueueFetchDataWithoutPendingData(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 1000)

	job1 := dequeue(t, q)
	assert.Equal(t, job1.payload, &HeadersJob{1, 190})
	assert.NoError(t, q.deliverHeaders(job1.id, headers[1:191]))

	// Get one completed element and reset the head hash of the queue
	assert.Len(t, q.FetchCompletedData(), 1)
	assert.Equal(t, q.head, headers[190].Hash())

	job2 := dequeue(t, q)
	assert.Equal(t, job2.payload, &HeadersJob{191, 190})

	// Wrong head hash
	assert.Error(t, q.deliverHeaders(job2.id, headers[102:200]))
	// Correct head hash
	assert.NoError(t, q.deliverHeaders(job2.id, headers[191:200]))
}

func TestQueueFetchCompletedData(t *testing.T) {
	// it should update the head when we get the completed data
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 1000)

	job1 := dequeue(t, q)
	assert.Equal(t, job1.payload, &HeadersJob{1, 190})
	job2 := dequeue(t, q)
	assert.Equal(t, job2.payload, &HeadersJob{191, 190})
	job3 := dequeue(t, q)
	assert.Equal(t, job3.payload, &HeadersJob{381, 190})

	assert.NoError(t, q.deliverHeaders(job1.id, headers[1:191]))
	assert.NoError(t, q.deliverHeaders(job2.id, headers[191:381]))

	// Fetch new data and reset head
	assert.Len(t, q.FetchCompletedData(), 2)
	assert.Equal(t, q.head, headers[380].Hash())

	job4 := dequeue(t, q)
	assert.Equal(t, job4.payload, &HeadersJob{571, 190})

	// Deliver wrong result for job3
	assert.Error(t, q.deliverHeaders(job3.id, headers[205:301]))

	// Deliver good result for job3 and job4
	assert.NoError(t, q.deliverHeaders(job3.id, headers[381:571]))
	assert.NoError(t, q.deliverHeaders(job4.id, headers[571:761]))
}

func TestQueueDeliverHeaderInWrongOrder(t *testing.T) {
	// if the wrong headers are delivered, it should be catched by the chain state
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 1000)

	job1 := dequeue(t, q)
	assert.Equal(t, job1.payload, &HeadersJob{1, 190})
	elem1 := findElement(t, q, job1.id)

	job2 := dequeue(t, q)
	assert.Equal(t, job2.payload, &HeadersJob{191, 190})
	elem2 := findElement(t, q, job2.id)

	job3 := dequeue(t, q)
	assert.Equal(t, job3.payload, &HeadersJob{381, 190})
	elem3 := findElement(t, q, job3.id)

	// deliver job1 with wrong content
	assert.Error(t, q.deliverHeaders(job1.id, headers[2:102]))

	assert.NoError(t, q.deliverHeaders(job3.id, headers[381:571]))
	assert.NoError(t, q.deliverHeaders(job1.id, headers[1:191]))

	// deliver job2 with wrong chain on the front
	assert.Error(t, q.deliverHeaders(job2.id, headers[192:381]))
	assert.NoError(t, q.deliverHeaders(job2.id, headers[191:381]))

	assert.True(t, elem1.Completed())
	assert.True(t, elem2.Completed())
	assert.True(t, elem3.Completed())
}

func TestQueueDeliverTotalHeaders(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 100)

	job := dequeue(t, q)
	assert.Equal(t, job.payload, &HeadersJob{1, 99})

	elem := findElement(t, q, job.id)
	assert.Equal(t, elem.headersStatus, pendingX)

	assert.NoError(t, q.deliverHeaders(job.id, headers[1:100]))
	assert.Equal(t, elem.headersStatus, completedX)
	assert.True(t, elem.Completed())
}

func headersToHashes(headers []*types.Header) []common.Hash {
	res := []common.Hash{}
	for _, i := range headers {
		res = append(res, i.Hash())
	}
	return res
}

func TestQueueDeliverReceiptsAndBodies(t *testing.T) {
	headers, blocks, receipts := blockchain.NewTestBodyChain(1000)
	q := newTestQueue(headers[0], 1000)

	bodies := []*types.Body{}
	for _, block := range blocks {
		bodies = append(bodies, block.Body())
	}

	// ask for headers from 1 to 190
	job := dequeue(t, q)
	assert.Equal(t, job.payload, &HeadersJob{1, 190})

	elem := findElement(t, q, job.id)
	assert.NoError(t, q.deliverHeaders(job.id, headers[1:191]))
	assert.False(t, elem.Completed())

	assert.Equal(t, elem.receiptsStatus, waitingX)
	assert.Equal(t, elem.bodiesStatus, waitingX)
	assert.Equal(t, elem.headersStatus, completedX)

	receiptsJob := dequeue(t, q)

	hashes1 := headersToHashes(headers[1:191])
	assert.Equal(t, receiptsJob.payload.(*ReceiptsJob).hashes, hashes1)
	assert.Equal(t, elem.receiptsStatus, pendingX)

	// dequeue another job for bodies at the same time
	bodiesJob := dequeue(t, q)
	assert.Equal(t, bodiesJob.payload.(*BodiesJob).hashes, hashes1)
	assert.Equal(t, elem.bodiesStatus, pendingX)

	// deliver half the receipts
	assert.NoError(t, q.deliverReceipts(receiptsJob.id, receipts[1:50]))
	assert.Equal(t, elem.receiptsOffset, uint32(49))

	hashes2 := headersToHashes(headers[50:191])
	receiptsJob = dequeue(t, q)
	assert.Equal(t, receiptsJob.payload.(*ReceiptsJob).hashes, hashes2)

	// Deliver incorrect receipts first
	assert.Error(t, q.deliverReceipts(receiptsJob.id, receipts[1:50]))
	// Deliver correct receipts
	assert.NoError(t, q.deliverReceipts(receiptsJob.id, receipts[50:191]))
	assert.Equal(t, elem.receiptsOffset, uint32(0))
	assert.Equal(t, elem.receiptsStatus, completedX)

	// Bodies are still pending, next job is a headers query
	job = dequeue(t, q)
	assert.Equal(t, job.payload, &HeadersJob{191, 190})

	// Deliver wrong bodies
	assert.Error(t, q.deliverBodies(bodiesJob.id, bodies[51:60]))
	// Deliver correct number of bodies
	assert.NoError(t, q.deliverBodies(bodiesJob.id, bodies[1:50]))
	assert.Equal(t, elem.bodiesOffset, uint32(49))

	bodiesJob = dequeue(t, q)
	assert.Equal(t, bodiesJob.payload.(*BodiesJob).hashes, hashes2)

	assert.NoError(t, q.deliverBodies(bodiesJob.id, bodies[50:191]))
	assert.Equal(t, elem.bodiesOffset, uint32(0))
	assert.Equal(t, elem.bodiesStatus, completedX)

	assert.True(t, elem.Completed())
}

func TestQueueMemoryRelease(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 1000)

	job := dequeue(t, q)
	assert.Equal(t, job.payload, &HeadersJob{1, 190})
	assert.NoError(t, q.deliverHeaders(job.id, headers[1:191]))

	job2 := dequeue(t, q)
	completed := q.FetchCompletedData()

	assert.Len(t, completed, 1)
	assert.Nil(t, completed[0].next)

	elem := findElement(t, q, job2.id)
	assert.Nil(t, elem.prev)
}
