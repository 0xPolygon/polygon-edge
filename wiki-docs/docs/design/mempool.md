## What is a Mempool?

A mempool, short for "memory pool," is a temporary storage area in a blockchain node for pending transactions before they are added to a block and confirmed on the network. When users submit transactions, they are initially held in the mempool, where miners or validators can select them to include in the next block. Transactions in the mempool are generally ordered based on factors such as transaction fees and age, which helps prioritize transactions for faster confirmation.

## Mempool in Edge

In Edge, the mempool is crucial in managing and prioritizing pending transactions. When a transaction is submitted to the network, it is first stored in the mempool, which remains until it is confirmed in a block. The mempool helps ensure that transactions are executed fairly and efficiently, as validators can choose which transactions to include in a block based on factors such as fees and age.

The mempool in Edge is designed to optimize transaction processing and improve network performance. Some of the key features of the mempool in Edge include:

- **Transaction prioritization**: The mempool prioritizes transactions based on fees and age. Transactions with higher fees are generally processed faster, providing greater incentives for validators to include them in a block.

- **Mempool management**: Mechanisms for handling congestion, transaction eviction, and memory usage are implemented to help maintain a healthy mempool, ensuring that the network remains stable and responsive.

- **Transaction validation**: Before a transaction is added to the mempool, validators validate it to ensure that it is valid and adheres to the network's consensus rules.

- **Mempool synchronization**: Nodes share information about their mempools to maintain a consistent view of the pending transactions in the network.
