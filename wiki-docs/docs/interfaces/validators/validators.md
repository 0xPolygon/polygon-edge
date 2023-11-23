The `IValidator` interface represents a validator and its reward pool for delegators on the childchain. This user guide will explain the structs that make up the interface.

## Structs

### RewardPool

RewardPool represents a pool for reward distribution, formed by delegators to a specific validator. It tracks slashed delegations using virtual balances.

- `supply` (uint256): The amount of tokens in the pool.
- `virtualSupply` (uint256): The total supply of virtual balances in the pool.
- `magnifiedRewardPerShare` (uint256): A coefficient to aggregate rewards.
- `validator` (address): The address of the validator the pool is based on.
- `magnifiedRewardCorrections` (mapping): Adjustments to reward magnifications by address.
- `claimedRewards` (mapping): The amount claimed by address.
- `balances` (mapping): The virtual balance by address.

### Validator

Validator represents a validator on the childchain.

- `blsKey` (uint256[4]): The public BLS key of the validator.
- `stake` (uint256): The amount staked by the validator.
- `commission` (uint256): The fee taken from delegators' rewards and given to the validator.
- `withdrawableRewards` (uint256): The amount that can be withdrawn.
- `active` (bool): Indicates if the validator is actively proposing/attesting.

### Node

Node represents a node in the red-black validator tree.

- `parent` (address): The address of the parent of this node.
- `left` (address): The node in the tree to the left of this one.
- `right` (address): The node in the tree to the right of this one.
- `red` (bool): A boolean denoting the color of the node for balancing.
- `validator` (Validator): The validator data type.

### ValidatorTree

ValidatorTree represents a red-black validator tree.

- `root` (address): The root of the tree.
- `count` (uint256): The number of nodes in the tree.
- `totalStake` (uint256): The total amount staked by nodes of the tree.
- `nodes` (mapping): A mapping from an address to a Node.
- `delegationPools` (mapping): A mapping from a validator address to a RewardPool.
