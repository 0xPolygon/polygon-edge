This document presents a summary of performance benchmarks for Edge production releases.

## Summary of Test Results

### v1.0.0-rc

| Transaction Type | TPS Sent | TPS Mined | Gas per Second | Gas per Transaction | Instance Type(s)             |
|------------------|----------|-----------|----------------|---------------------|------------------------------|
| EOA to EOA       | 2,600    | 2,442     | 51,282,000     | 21,000              | x2iezn.2xlarge               |
| ERC20            | 1,300    | 591       | 21,696,201     | 36,711              | x2iezn.2xlarge               |
| ERC721           | 800      | 558       | 32,642,442     | 58,499              | x2iezn.2xlarge               |

### v1.0.0

| Transaction Type | TPS Sent | TPS Mined | Gas per Second | Gas per Transaction | Instance Type(s)             |
|------------------|----------|-----------|----------------|---------------------|------------------------------|
| EOA to EOA       | 2,475    | 2,459     | 51,639,000     | 21,000              | x2iezn.2xlarge               |
| ERC20            | 550      | 680       | 24,963,480     | 36,711              | x2iezn.2xlarge               |
| ERC721           | 550      | 649       | 37,965,851     | 58,499              | x2iezn.2xlarge               |

### v1.1.0

| Transaction Type | TPS Sent | TPS Mined | Gas per Second | Gas per Transaction | Instance Type(s)             |
|------------------|----------|-----------|----------------|---------------------|------------------------------|
| EOA to EOA       | 2,475    | 2,396     | 50,316,000     | 21,000              | x2iezn.2xlarge               |
| ERC20            | 550      | 653       | 23,972,283     | 36,711              | x2iezn.2xlarge               |
| ERC721           | 550      | 621       | 36,327,879     | 58,499              | x2iezn.2xlarge               |

### v1.3.0

| Transaction Type | TPS Sent | TPS Mined | Gas per Second | Gas per Transaction | Instance Type(s)             |
|------------------|----------|-----------|----------------|---------------------|------------------------------|
| EOA to EOA       | 3,000    | 2,577     | 54,117,000     | 21,000              | x2iezn.2xlarge               |
| ERC20            | 820      | 813       | 23,005,250     | 28,297              | x2iezn.2xlarge               |
| ERC721           | 750      | 746       | 37,393,367     | 50,125              | x2iezn.2xlarge               |

## Test Environment

The performance tests were conducted in a controlled environment to ensure accuracy and consistency in the results. The environment setup includes the following components:

### Network Configuration

- Network: Polygon Edge
- Consensus Algorithm: PolyBFT

### Instance Types

The tests utilized various Amazon Web Services (AWS) instance types to evaluate the impact of varying compute resources on network performance:

- **t2.xlarge**
- **t2.micro**
- **c5.2xlarge**
- **c6a.48xlarge**
- **x2iezn.2xlarge**
- **c6a.xlarge**

<details>
<summary>General instance specifications</summary>

- **t2.xlarge**
  - vCPU: 4
  - Memory: 16 GiB
  - Network Performance: Up to 5 Gigabit
  - EBS-Optimized: Up to 2,750 Mbps
- **t2.micro**
  - vCPU: 1
  - Memory: 1 GiB
  - Network Performance: Low to Moderate
  - EBS-Optimized: Not available
- **c5.2xlarge**
  - vCPU: 8
  - Memory: 16 GiB
  - Network Performance: Up to 10 Gigabit
  - EBS-Optimized: Up to 3,500 Mbps
- **c6a.48xlarge**
  - vCPU: 192
  - Memory: 768 GiB
  - Network Performance: 50 Gigabit
  - EBS-Optimized: 14,000 Mbps
- **x2iezn.2xlarge**
  - vCPU: 8
  - Memory: 64 GiB
  - Network Performance: Up to 25 Gigabit
  - EBS-Optimized: Up to 3,500 Mbps
- **c6a.xlarge**
  - vCPU: 4
  - Memory: 16 GiB
  - Network Performance: Up to 10 Gigabit
  - EBS-Optimized: Up to 4,750 Mbps

</details>

### Transaction Types

The tests were performed using three different types of transactions to assess the network's capability to handle various use cases:

- **EOA to EOA**: Representing simple value transfers between user accounts.
- **ERC20**: Token transfers, simulating the exchange of tokens on the network.
- **ERC721**: Non-fungible token (NFT) transfers representing the exchange of unique digital assets.

### Performance Metrics

The following key performance metrics were considered during the tests:

- **Transactions per second (TPS) sent**: The number of transactions submitted to the network per second.
- **Transactions per second (TPS) mined**: The number of transactions successfully processed and included in the blockchain per second.
- **Gas per transaction**: The amount of gas consumed by each transaction, representing the computational cost.
- **Gas per second**: The total amount consumed per second indicates the network's overall computational capacity.
