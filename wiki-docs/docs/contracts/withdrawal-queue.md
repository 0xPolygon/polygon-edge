## Overview

The `WithdrawalQueueLib` library provides functions for managing a queue of
withdrawals. The library defines two data types: `Withdrawal` and `WithdrawalQueue`.

## Data Types

### Withdrawal

The `Withdrawal` struct represents a withdrawal from the contract. It has the following fields:

- `amount`: The amount to withdraw.
- `epoch`: The epoch of the withdrawal.

### WithdrawalQueue

The `WithdrawalQueue` struct represents a queue of Withdrawal structs. It has the following fields:

- `head`: The earliest unprocessed index (the most recently filled withdrawal).
- `tail`: The index of the most recent withdrawal (the total number of submitted withdrawals).
- `withdrawals`: A mapping from index to Withdrawal struct, representing the withdrawals in the queue.

## Functions

### append

```js
function append(
    WithdrawalQueue storage self,
    uint256 amount,
    uint256 epoch
) internal
```

This function updates the `WithdrawalQueue` with new withdrawal data.
If there is already a withdrawal for the epoch being submitted, the `amount` will be added
to that `epoch`; otherwise, a new `Withdrawal` struct will be created in the queue.

The function takes the following arguments:

- `self`: The WithdrawalQueue struct represents the queue of withdrawals.
- `amount`: The amount to withdraw.
- `epoch`: The epoch of the withdrawal.

### withdrawable

```js
function withdrawable(
    WithdrawalQueue storage self,
    uint256 currentEpoch
) internal view returns (uint256 amount, uint256 newHead)
```

This function returns the amount withdrawable through a specified epoch and the new head index. It is meant to be used with the current epoch being passed in.

The function takes the following arguments:

- `self`: The WithdrawalQueue struct.
- `currentEpoch`: The epoch to check from.

The function returns a tuple containing the following:

- `amount`: The amount withdrawable through the specified epoch.
- `newHead`: The head of the queue once these withdrawals have been processed.

### pending

```js
function pending(
    WithdrawalQueue storage self,
    uint256 currentEpoch
) internal view returns (uint256 amount)
```

This function returns the amount withdrawable beyond a specified `epoch`.
It is meant to be used with the current epoch being passed in.

The function takes the following arguments:

- `self`: The WithdrawalQueue struct.
- `currentEpoch`: The epoch to check from.

The function returns the `amount` withdrawable from beyond the specified `epoch`.

### pop

```js
function pop(WithdrawalQueue storage self) internal
```

This function removes the earliest unprocessed withdrawal from the
`WithdrawalQueue`. It updates the head field of the `WithdrawalQueue`
struct to reflect this change.

The function takes the following argument:

- `self`: The `WithdrawalQueue` struct.

### clear

```js
function clear(WithdrawalQueue storage self) internal
```

This function removes all withdrawals from the `WithdrawalQueue`.
It resets the `head` and `tail` fields of the `WithdrawalQueue` struct to `0`.

The function takes the following argument:

- self: The `WithdrawalQueue` struct.
