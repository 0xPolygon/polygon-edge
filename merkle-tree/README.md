# Merkle Tree Implementation in Go

This repository contains an implementation of a Merkle tree in Go, along with benchmarking results. 

## Overview

A Merkle tree is a hash-based data structure that allows efficient and secure verification of the contents of large data sets. It is commonly used in peer-to-peer networks, blockchain systems, and other distributed systems. The tree is constructed by recursively hashing pairs of data elements until a single hash value is obtained, which is called the root hash. 

The main advantages of using a Merkle tree are its ability to efficiently verify data integrity, its compact representation of large data sets, and its support for partial data verification. 

## Implementation

The Merkle tree implementation in this repository is relatively simple and follows the basic structure of a binary tree. Each leaf node in the tree represents a data element, and each non-leaf node is the hash of its two child nodes. The root hash is the hash of the two child nodes of the root node. 

The implementation includes functions for constructing a Merkle tree from a set of data elements, computing the root hash of the tree, generating and verifying the proof of membership for a piece of data.

## Benchmarking

To benchmark the performance of the Merkle tree implementation, we used the Go testing package and the built-in benchmarking tools. We tested the implementation with different sizes of data sets, ranging from 10 elements to 1,000,000 elements. 

The benchmark results show that the Merkle tree implementation has a linear time complexity with respect to the size of the data set. The construction of the tree takes O(n log n) time, where n is the number of data elements. The verification of a single data element takes O(log n) time. 

Here are the benchmarking results for constructing a Merkle tree from a set of data elements:

| TestName | Number of leaves in tree | Results |
|----------|--------------------------|---------|
| Benchmark_MerkleTreeCreation_10-8 | 10 | 73446             17496 ns/op           14272 B/op        119 allocs/op |
| Benchmark_MerkleTreeCreation_100-8 | 100 | 7204            148145 ns/op          132744 B/op       1043 allocs/op |
| Benchmark_MerkleTreeCreation_1K-8 | 1000 | 715           1485988 ns/op         1305667 B/op      10064 allocs/op |
| Benchmark_MerkleTreeCreation_10K-8 | 10,000 | 74          15883932 ns/op        13130629 B/op     100141 allocs/op |
| Benchmark_MerkleTreeCreation_100K-8 | 100,000 | 7         147831662 ns/op        132476874 B/op   1000217 allocs/op |
| Benchmark_MerkleTreeCreation_1M-8 | 1,000,000 | 1        1538599360 ns/op        1329522960 B/op 10000325 allocs/op |

Here are the benchmarking results for generating a proof of membership for a piece of data in a Merkle tree:

| TestName | Number of leaves in tree | Results |
|----------|--------------------------|---------|
| Benchmark_GenerateProof_10-8 | 10 | 6827118               156.8 ns/op           224 B/op          3 allocs/op |
| Benchmark_GenerateProof_100-8 | 100 | 4569232               251.5 ns/op           480 B/op          4 allocs/op |
| Benchmark_GenerateProof_1K-8 | 1000 | 2740286               443.0 ns/op           992 B/op          5 allocs/op |
| Benchmark_GenerateProof_10K-8 | 10,000 | 2008087               521.0 ns/op           992 B/op          5 allocs/op |
| Benchmark_GenerateProof_100K-8 | 100,000 | 1252176               840.1 ns/op          2016 B/op          6 allocs/op |
| Benchmark_GenerateProof_1M-8 | 1,000,000 | 1550371               793.2 ns/op          2016 B/op          6 allocs/op |

Here are the benchmarking results for verifying a proof of membership for a piece of data in a Merkle tree:

| TestName | Number of leaves in tree | Results |
|----------|--------------------------|---------|
| Benchmark_VerifyProof_10-8 | 10 | 299299              3485 ns/op            3136 B/op         20 allocs/op |
| Benchmark_VerifyProof_100-8 | 100 | 216274              5620 ns/op            4768 B/op         32 allocs/op |
| Benchmark_VerifyProof_1K-8 | 1000 | 151994              7340 ns/op            6400 B/op         44 allocs/op |
| Benchmark_VerifyProof_10K-8 | 10,000 | 113112             10024 ns/op            8576 B/op         60 allocs/op |
| Benchmark_VerifyProof_100K-8 | 100,000 | 97400             12088 ns/op           10208 B/op         72 allocs/op |
| Benchmark_VerifyProof_1M-8 | 1,000,000 | 95178             13368 ns/op           11840 B/op         84 allocs/op |

As you can see, the construction of the Merkle tree takes longer than verifying a single data element. However, both operations have a reasonable time complexity, even for large data sets. 

## Conclusion

This Merkle tree implementation in Go provides a simple and efficient way to verify the integrity of large data sets. The implementation is easy to understand and customize, and the benchmarking results show that it performs well for data sets of different sizes.
