## bridge_generateExitProof

Returns the proof for a given exit event. Used by users that want to exit the L2 chain and withdraw tokens to L1.

### Parameters

**exitID** - ID of the exit event submitted by L2StateSender contract when the user wants to exit the L2 contract.

### Returns


- **Object** - A proof object containing:
  - **Array of hashes** - representing the proof of membership of a given exit event on some checkpoint.
  - **Map** - containing a leaf index of given exit event in the Merkle tree, Exit event and block number in which checkpoint that contains a given exit event was submitted to the rootchain.

---

## bridge_getStateSyncProof

Returns the proof for a given state sync event. Used by users (Relayer) when they want to execute a state change on the childchain (for example, depositing funds from L1 to L2).

### Parameters

**stateSyncID** - ID of the state sync event submitted by StateSender contract.

### Returns


- **Object** - A proof object containing:
  - **Array of hashes** - representing the proof of membership of a given state sync event on some commitment.
  - **Map** - containing the state sync event data.
