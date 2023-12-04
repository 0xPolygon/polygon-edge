Edge-powered chains are built on a common blockchain design that effectively manages and maintains the blockchain data structure, consisting of a sequential chain of blocks containing transactions and other metadata; a state database.

The core blockchain implementation offers various functionalities, such as:

- Adding new blocks to the chain.
- Retrieving blocks using their hash or number.
- Handling chain reorganizations (i.e., switching to a different chain with greater difficulty).
- Verifying block headers and gas limits.
- Caching block headers and receipts to improve retrieval speed.
- Updating the chain's average gas price.

To provide these functionalities, the implementation employs several sub-components, including:

- A consensus component responsible for validating new blocks and incorporating them into the chain.
- A database component that persistently stores blockchain data.
- An event component that notifies other components of chain reorganizations and new block additions.
- A transaction signer component that verifies transaction signatures and identifies the sender address.
- A gas price calculator component that computes the chain's average gas price.
