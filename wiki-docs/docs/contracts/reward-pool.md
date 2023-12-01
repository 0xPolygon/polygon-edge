## Overview

`RewardPool` is library for the management and distribution of validator rewards.
Each validator has a reward pool for delegators delegating funds to them.

## Functions

### distributeReward

```js
function distributeReward(RewardPool storage pool, uint256 amount) internal {
    if (amount == 0) return;
    if (pool.supply == 0) revert NoTokensDelegated(pool.validator);
    pool.magnifiedRewardPerShare += (amount * magnitude()) / pool.supply;
}
```

This function distributes an amount of rewards to the `pool`.
If amount is 0 or `pool.supply` is 0, this function does nothing.
Otherwise, `pool.magnifiedRewardPerShare` is incremented by amount multiplied
by `magnitude` divided by `pool.supply`.

### deposit

```js
function deposit(
    RewardPool storage pool,
    address account,
    uint256 amount
) internal {
    pool.balances[account] += amount;
    pool.supply += amount;
    pool.magnifiedRewardCorrections[account] -= (pool.magnifiedRewardPerShare * amount).toInt256Safe();
}
```

This function credits the balance of account in the `pool` by amount.
`pool.balances[account]` is incremented by amount, `pool.supply`
is incremented by amount, and `pool.magnifiedRewardCorrections[account]` is
decremented by amount multiplied by `pool.magnifiedRewardPerShare` after being
converted to an int256 using `toInt256Safe`.

### withdraw

```js
function withdraw(
    RewardPool storage pool,
    address account,
    uint256 amount
) internal {
    pool.balances[account] -= amount;
    pool.supply -= amount;
    pool.magnifiedRewardCorrections[account] += (pool.magnifiedRewardPerShare * amount).toInt256Safe();
}
```

This function decrements the balance of account in the `pool` by
amount. `pool.balances[account]` is decremented by amount, `pool.supply` is
decremented by amount, and `pool.magnifiedRewardCorrections[account]` is
incremented by amount multiplied by `pool.magnifiedRewardPerShare` after being
converted to an int256 using `toInt256Safe()`.

### claimRewards

```js
function claimRewards(RewardPool storage pool, address account) internal returns (uint256 reward) {
    reward = claimableRewards(pool, account);
    pool.claimedRewards[account] += reward;
}
```

This function increments `pool.claimedRewards[account]` by the claimable rewards
for account in the `pool`, and returns the amount of rewards claimed. The claimable
rewards for account can be calculated using `claimableRewards` .

### balanceOf

```js
function balanceOf(RewardPool storage pool, address account) internal view returns (uint256) {
    return pool.balances[account];
}
```

This function returns the `balance` (i.e. amount delegated) of account in the `pool`.

### totalRewardsEarned

```js
function totalRewardsEarned(RewardPool storage pool, address account) internal view returns (uint256) {
    int256 magnifiedRewards = (pool.magnifiedRewardPerShare * pool.balances[account]).toInt256Safe();
    uint256 correctedRewards = (magnifiedRewards + pool.magnifiedRewardCorrections[account]).toUint256Safe();
    return correctedRewards / magnitude();
}
```

This function returns the total rewards earned by account in the `pool`.
This is calculated by first multiplying `pool.magnifiedRewardPerShare` by
`pool.balances[account]`, converting the result to an int256 using `toInt256Safe`,
and then adding `pool.magnifiedRewardCorrections[account]`.
The final result is then divided by `magnitude` and returned as a `uint256`.

### claimableRewards

```js
function claimableRewards(RewardPool storage pool, address account) internal view returns (uint256) {
    return totalRewardsEarned(pool, account).sub(pool.claimedRewards[account]);
}
```

This function returns the amount of claimable rewards for account in the pool.
This is calculated by subtracting `pool.claimedRewards[account]` from `totalRewardsEarned`.

### magnitude

```js
function magnitude() private pure returns (uint256) {
        return 1e18;
    }
```

This function returns the `magnitude` of the reward multiplier.
