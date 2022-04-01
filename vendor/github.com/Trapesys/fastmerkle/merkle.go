package fastmerkle

import (
	"errors"
	"fmt"
)

var (
	errEmptyDataSet = errors.New("empty data set provided")
)

// GenerateMerkleTree generates a Merkle tree based on the input data
func GenerateMerkleTree(inputData [][]byte) (*MerkleTree, error) {
	// Check if the data set is valid
	if len(inputData) < 1 {
		return nil, errEmptyDataSet
	}

	// Create the worker pool and put them on standby
	workerPool := newWorkerPool(len(inputData) + 1)

	defer workerPool.close()

	// Generate the leaves of the Merkle tree
	nodes, leafErr := generateLeaves(inputData, workerPool)
	if leafErr != nil {
		return nil, fmt.Errorf(
			"unable to generate leaf nodes, %w",
			leafErr,
		)
	}

	// While the root is not derived, create new hashing jobs
	// for the worker pool
	for len(nodes) > 1 {
		// Make sure the input node array for the level is even
		nodes = adjustLevelSize(nodes)

		// A hashing job is just hashing two subsequent
		// siblings in the tree. Since the tree is a perfect
		// Merkle tree, the node array for the level will always be even
		for i := 0; i < len(nodes); i += 2 {
			workerPool.addJob(&workerJob{
				storeIndex: i,
				sourceData: [][]byte{
					nodes[i].hash,
					nodes[i+1].hash,
				},
			})
		}

		// The Merkle tree is being built from bottom to top,
		// so each level has exactly 1/2 fewer nodes
		// than the previous level (property of perfect binary trees).
		// Therefore, for N nodes on a single tree level, only N/2 results can be expected
		for i := 0; i < len(nodes)/2; i++ {
			result := workerPool.getResult()
			if result.error != nil {
				return nil, fmt.Errorf(
					"unable to perform hashing, %w",
					result.error,
				)
			}

			// Create a placeholder for the parent node
			parent := &Node{
				// Save the hashing data of the 2 children
				hash: result.hashData,
				// Save a reference to the left child
				left: nodes[result.storeIndex],
				// Save a reference to the right child
				right: nodes[result.storeIndex+1],
			}

			// Save the parent reference with the children
			nodes[result.storeIndex].parent = parent
			nodes[result.storeIndex+1].parent = parent

			// Overwrite the left child's slot in the array,
			// since it's no longer needed. The right child
			// is also not needed anymore in the original array,
			// and will be overwritten later
			nodes[result.storeIndex] = parent
		}

		// Now that results are gathered for the level,
		// the array can be shifted and shrunk
		nodes = packLevelResults(nodes)
	}

	return &MerkleTree{
		root: nodes[0],
	}, nil
}

// packLevelResults shifts every other node to the
// beginning of the array, and discards half of it (shrinks it).
// Due to the way results are being stored (index of left child),
// and the fact that the Merkle tree is a perfect binary tree,
// it can be guaranteed that the results are on every other index in the node level array
func packLevelResults(nodes []*Node) []*Node {
	// Put the results in the first half of the array.
	// One counter keeps track of the next slot to place the value (moves by 1)
	// and the other keeps track of which element should be stored (moves by 2)
	for i := 0; i < len(nodes)/2; i++ {
		nodes[i] = nodes[2*i]
	}

	// Wipe the other half of the array, since
	// all useful and needed results are in the first half
	return nodes[:len(nodes)/2]
}

// generateLeaves generates the initial (leaf) level of the Merkle tree.
// The leaf level needs to be a power of 2, since the Merkle tree is considered
// to be a perfect binary tree
func generateLeaves(inputData [][]byte, wp *workerPool) ([]*Node, error) {
	leaves := make([]*Node, len(inputData))

	// Create the initial job set for the leaf nodes,
	// where each job is a single leaf node to be processed
	for i, input := range inputData {
		wp.addJob(&workerJob{
			storeIndex: i,
			sourceData: [][]byte{
				input,
			},
		})
	}

	// Grab the results from the worker pool
	for range inputData {
		result := wp.getResult()
		if result.error != nil {
			return nil, fmt.Errorf(
				"unable to perform hashing, %w",
				result.error,
			)
		}

		// Save the leaf nodes
		leaves[result.storeIndex] = &Node{
			hash:   result.hashData,
			left:   nil,
			right:  nil,
			parent: nil,
		}
	}

	// Make sure the node array for the level is even
	return adjustLevelSize(leaves), nil
}

// adjustLevelSize checks if the input array is odd,
// and if it is, duplicates the last element to make it even
func adjustLevelSize(nodes []*Node) []*Node {
	if len(nodes)%2 == 0 {
		// The node array for the level is already even,
		// no need to do further processing
		return nodes
	}

	// Duplicate the last node in the level and append it
	lastNode := nodes[len(nodes)-1]

	return append(nodes, lastNode.duplicate())
}
