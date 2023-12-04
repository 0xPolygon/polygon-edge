## What is the TxPool used for?

The TxPool module manages the incoming transactions for processing. It maintains a list of unprocessed transactions and ensures they conform to specific constraints before entering the pool.

## TxPool in Edge

The TxPool has several methods to handle incoming transactions.

!!! info "Details on how the TxPool works"

    The `addTx()` method is the main entry point for all new transactions. It validates the incoming transaction and adds it to the pool. If the call is successful, an account is created for this address (only once), and an `enqueueRequest` is signaled.

    The `handleEnqueueRequest()` method attempts to enqueue the transaction in the given request to the associated account. If the account is eligible for promotion, a `promoteRequest` is signaled afterward. The `handlePromoteRequest()` method moves promotable transactions of some accounts from enqueued to promoted. It can only be invoked by `handleEnqueueRequest()` or `resetAccount()`.

    The TxPool also has a `validateTx()` method that checks specific constraints before entering the pool. These constraints include checking the transaction size to overcome DOS Attacks, ensuring the transaction has a strictly positive value, and checking if the transaction is appropriately signed.

    The `resetAccounts()` method also updates existing accounts with the new nonce and prunes stale transactions. This ensures that the pool only contains relevant and valid transactions. The `updateAccountSkipsCounts()` method updates the accounts' skips, which is the number of consecutive blocks that do not have the account's transactions.

    The TxPool implementation in Polygon Edge uses several data structures to manage transactions efficiently. The `accounts` data structure stores all the accounts and transactions in the pool. The `index` data structure maintains a list of all transactions in the pool, and the `gauge` data structure keeps track of the number of slots used in the pool.

    Finally, the TxPool implementation in Polygon Edge handles transactions gossiped by the network through the `addGossipTx()` method. It verifies that the gossiped transaction message is not empty, decodes the transaction, and adds it to the pool using the `addTx()` method.
