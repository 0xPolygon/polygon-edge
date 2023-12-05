The `IStateSender` interface provides a way to send state updates from one contract to another, often between different layers of a network, such as L1 and L2. This user guide will help you understand how to interact with the functions provided by the `IStateSender` interface.

## Functions

### syncState()

This function sends a state update to a specified contract, allowing the receiving contract to process the changes in the state.

#### Parameters

- `receiver` (address): The contract address will receive the state update.
- `data` (bytes): The calldata associated with the state update to be sent.

#### Usage

You'll need to call the syncState() function to send a state update from your contract. This function will send the state update to the specified receiver contract.

For example, you might use the `syncState()` function as follows:

```solidity
contract MyStateSender is IStateSender {
    function sendStateUpdate(address receiver, bytes calldata data) public {
        // Send the state update to the specified receiver contract
        syncState(receiver, data);
    }
}
```

This function enables your contract to communicate state updates to other contracts, facilitating cross-contract and cross-layer communication. By implementing the IStateSender interface, you can ensure your contract can send state updates to other parts of the ecosystem.
