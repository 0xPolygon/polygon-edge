package syncer

import (
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/blockchain"
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

func checkStatus(t *testing.T, status elementStatus, expected elementStatus) {
	if status != expected {
		t.Fatalf("expected status %s but found %s", expected.String(), status.String())
	}
}

func TestQueueDeliverPartialHeaders(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 100)

	job := dequeue(t, q)
	if !reflect.DeepEqual(job.payload, &HeadersJob{1, 100}) {
		t.Fatal("bad.1")
	}
	elem := findElement(t, q, job.id)

	checkStatus(t, elem.headersStatus, pendingX)

	if err := q.deliverHeaders(job.id, headers[1:90]); err != nil {
		t.Fatal(err)
	}
	checkStatus(t, elem.headersStatus, waitingX)

	job = dequeue(t, q)
	if !reflect.DeepEqual(job.payload, &HeadersJob{90, 11}) {
		t.Fatal("bad.2")
	}

	// deliver partial headers with wrong order
	if err := q.deliverHeaders(job.id, headers[91:101]); err == nil {
		t.Fatal("it should fail")
	}
	if err := q.deliverHeaders(job.id, headers[90:100]); err != nil {
		t.Fatal(err)
	}
	checkStatus(t, elem.headersStatus, completedX)

	if !elem.Completed() {
		t.Fatal("it should be completed")
	}
}

func TestQueueReceiptsAndBodiesCompletedByDefault(t *testing.T) {
	// if we are pending a headers job, the next job to dequeue has to be
	// for another headersjob

	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 1000)

	job1 := dequeue(t, q)
	if !reflect.DeepEqual(job1.payload, &HeadersJob{1, 100}) {
		t.Fatal("bad.1")
	}
	elem1 := findElement(t, q, job1.id)

	checkStatus(t, elem1.receiptsStatus, completedX)
	checkStatus(t, elem1.bodiesStatus, completedX)
	checkStatus(t, elem1.headersStatus, pendingX)

	job2 := dequeue(t, q)
	if !reflect.DeepEqual(job2.payload, &HeadersJob{101, 100}) {
		t.Fatal("bad.2")
	}
	elem2 := findElement(t, q, job2.id)

	if err := q.deliverHeaders(job2.id, headers[101:201]); err != nil {
		t.Fatal(err)
	}
	if err := q.deliverHeaders(job1.id, headers[1:101]); err != nil {
		t.Fatal(err)
	}

	if !elem1.Completed() {
		t.Fatal("batch 1 should be completed")
	}
	if !elem2.Completed() {
		t.Fatal("batch 2 should be completed")
	}
}

func TestQueueFetchDataWithoutPendingData(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 1000)

	job1 := dequeue(t, q)
	if !reflect.DeepEqual(job1.payload, &HeadersJob{1, 100}) {
		t.Fatal("bad.1")
	}
	if err := q.deliverHeaders(job1.id, headers[1:101]); err != nil {
		t.Fatal(err)
	}

	if elements := q.FetchCompletedData(); len(elements) != 1 {
		t.Fatal("it should have one element")
	}
	if q.head.String() != headers[100].Hash().String() {
		t.Fatal("head has not been reseted correctly")
	}

	job2 := dequeue(t, q)
	if !reflect.DeepEqual(job2.payload, &HeadersJob{101, 100}) {
		t.Fatal("bad.1")
	}
	if err := q.deliverHeaders(job2.id, headers[102:200]); err == nil {
		t.Fatal(err)
	}
	if err := q.deliverHeaders(job2.id, headers[101:200]); err != nil {
		t.Fatal(err)
	}
}

func TestQueueFetchCompletedData(t *testing.T) {
	// it should update the head when we get the completed data
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 1000)

	job1 := dequeue(t, q)
	if !reflect.DeepEqual(job1.payload, &HeadersJob{1, 100}) {
		t.Fatal("bad.1")
	}
	job2 := dequeue(t, q)
	if !reflect.DeepEqual(job2.payload, &HeadersJob{101, 100}) {
		t.Fatal("bad.2")
	}
	job3 := dequeue(t, q)
	if !reflect.DeepEqual(job3.payload, &HeadersJob{201, 100}) {
		t.Fatal("bad.3")
	}

	if err := q.deliverHeaders(job1.id, headers[1:101]); err != nil {
		t.Fatal(err)
	}
	if err := q.deliverHeaders(job2.id, headers[101:201]); err != nil {
		t.Fatal(err)
	}

	if q.NumOfCompletedBatches() != 2 {
		t.Fatal("it should have 2 batches completed")
	}

	q.FetchCompletedData()
	if q.head.String() != headers[200].Hash().String() {
		t.Fatal("head has not been reseted correctly")
	}

	job4 := dequeue(t, q)
	if !reflect.DeepEqual(job4.payload, &HeadersJob{301, 100}) {
		t.Fatal("bad.4")
	}

	// deliver with wrong header
	if err := q.deliverHeaders(job3.id, headers[205:301]); err == nil {
		t.Fatal("it should have failed")
	}

	if err := q.deliverHeaders(job3.id, headers[201:301]); err != nil {
		t.Fatal(err)
	}
	if err := q.deliverHeaders(job4.id, headers[301:401]); err != nil {
		t.Fatal(err)
	}
}

func TestQueueDeliverHeaderInWrongOrder(t *testing.T) {
	// if the wrong headers are delivered, it should be catched by the chain state
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 1000)

	job1 := dequeue(t, q)
	if !reflect.DeepEqual(job1.payload, &HeadersJob{1, 100}) {
		t.Fatal("bad.1")
	}
	elem1 := findElement(t, q, job1.id)

	job2 := dequeue(t, q)
	if !reflect.DeepEqual(job2.payload, &HeadersJob{101, 100}) {
		t.Fatal("bad.2")
	}
	elem2 := findElement(t, q, job2.id)

	job3 := dequeue(t, q)
	if !reflect.DeepEqual(job3.payload, &HeadersJob{201, 100}) {
		t.Fatal("bad.3")
	}
	elem3 := findElement(t, q, job3.id)

	// deliver job1 with wrong content
	if err := q.deliverHeaders(job1.id, headers[2:102]); err == nil {
		t.Fatal("it should fail")
	}

	if err := q.deliverHeaders(job3.id, headers[201:301]); err != nil {
		t.Fatal(err)
	}
	if err := q.deliverHeaders(job1.id, headers[1:101]); err != nil {
		t.Fatal(err)
	}

	// deliver job2 with wrong chain on the front
	if err := q.deliverHeaders(job2.id, headers[102:201]); err == nil {
		t.Fatal("it should fail")
	}
	if err := q.deliverHeaders(job2.id, headers[101:201]); err != nil {
		t.Fatal(err)
	}

	if !elem1.Completed() {
		t.Fatal("batch 1 should be completed")
	}
	if !elem2.Completed() {
		t.Fatal("batch 1 should be completed")
	}
	if !elem3.Completed() {
		t.Fatal("batch 1 should be completed")
	}
}

func TestQueueDeliverTotalHeaders(t *testing.T) {
	headers := blockchain.NewTestHeaderChain(1000)
	q := newTestQueue(headers[0], 100)

	job := dequeue(t, q)
	if !reflect.DeepEqual(job.payload, &HeadersJob{1, 100}) {
		t.Fatal("bad.1")
	}

	elem := findElement(t, q, job.id)
	checkStatus(t, elem.headersStatus, pendingX)

	if err := q.deliverHeaders(job.id, headers[1:100]); err != nil {
		t.Fatal(err)
	}
	checkStatus(t, elem.headersStatus, completedX)

	if !elem.Completed() {
		t.Fatal("it should be completed")
	}
}

func TestQueueDeliverReceiptsAndBodies(t *testing.T) {
	headers, blocks, receipts := blockchain.NewTestBodyChain(1000)
	q := newTestQueue(headers[0], 1000)

	bodies := []*types.Body{}
	for _, block := range blocks {
		bodies = append(bodies, block.Body())
	}

	// ask for headers from 1 to 100
	job := dequeue(t, q)

	elem := findElement(t, q, job.id)
	if err := q.deliverHeaders(job.id, headers[1:101]); err != nil {
		t.Fatal(err)
	}

	if elem.Completed() {
		t.Fatal("receipts and bodies have not been fetched yet")
	}
	checkStatus(t, elem.receiptsStatus, waitingX)
	checkStatus(t, elem.bodiesStatus, waitingX)
	checkStatus(t, elem.headersStatus, completedX)

	receiptsJob := dequeue(t, q)
	if !reflect.DeepEqual((receiptsJob.payload.(*ReceiptsJob)).hashes, headers[1:101]) {
		t.Fatal("bad receipts job")
	}
	checkStatus(t, elem.receiptsStatus, pendingX)

	// dequeue another job for bodies at the same time
	bodiesJob := dequeue(t, q)
	if !reflect.DeepEqual((bodiesJob.payload.(*BodiesJob)).hashes, headers[1:101]) {
		t.Fatal("bad bodies job")
	}
	checkStatus(t, elem.bodiesStatus, pendingX)

	// deliver half the receipts
	if err := q.deliverReceipts(receiptsJob.id, receipts[1:50]); err != nil {
		t.Fatal(err)
	}
	if elem.receiptsOffset != 49 {
		t.Fatal("offset should be 49")
	}

	receiptsJob = dequeue(t, q)
	if !reflect.DeepEqual((receiptsJob.payload.(*ReceiptsJob)).hashes, headers[50:101]) {
		t.Fatal("bad receipts job")
	}
	// deliver incorrect receipts
	if err := q.deliverReceipts(receiptsJob.id, receipts[1:50]); err == nil {
		t.Fatal("incorrect receipts delivered, it should have failed")
	}
	if err := q.deliverReceipts(receiptsJob.id, receipts[50:101]); err != nil {
		t.Fatal(err)
	}
	checkStatus(t, elem.receiptsStatus, completedX)

	// bodies still pending, dequeue another job, has to be one for headers
	job = dequeue(t, q)
	if !reflect.DeepEqual(job.payload, &HeadersJob{101, 100}) {
		t.Fatal("bad headers jobs")
	}

	// deliver bodies in parts as well
	// make it fail first
	if err := q.deliverBodies(bodiesJob.id, bodies[51:60]); err == nil {
		t.Fatal("it should have failed")
	}
	if err := q.deliverBodies(bodiesJob.id, bodies[1:50]); err != nil {
		t.Fatal(err)
	}
	if elem.receiptsOffset != 49 {
		t.Fatal("offset should be 49")
	}
	bodiesJob = dequeue(t, q)
	if !reflect.DeepEqual((bodiesJob.payload.(*BodiesJob)).hashes, headers[50:101]) {
		t.Fatal("bad receipts job")
	}
	if err := q.deliverBodies(bodiesJob.id, bodies[50:101]); err != nil {
		t.Fatal(err)
	}

	// now is completed
	if !elem.Completed() {
		t.Fatal("it should be completed now")
	}
}

// Test combined, what happenes after u receive the other one
// Test to check the headers
// check after you commit the data the head changes
