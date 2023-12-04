The `IBLS` interface provides a set of functions for working with BLS (Boneh-Lynn-Shacham) signatures in a smart contract. This user guide will explain how to interact with the functions provided by the IBLS interface.

## Functions

### verifySingle()

This function verifies a single signature.

#### Parameters

- `signature` (uint256[2]): A 64-byte G1 group element (small signature).
- `pubkey` (uint256[4]): A 128-byte G2 group element (big pubkey).
- `message` (uint256[2]): The message signed to produce the signature.

#### Usage

To verify a single signature, call the verifySingle() function with the required parameters:

```solidity
(bool sigVerification, bool callSuccess) = IBLS.instance.verifySingle(signature, pubkey, message);
```

### verifyMultiple()

This function verifies multiple non-aggregated signatures where each message is unique.

#### Parameters

- `signature` (uint256[2]): A 64-byte G1 group element (small signature).
- `pubkeys` (uint256[4][]): An array of 128-byte G2 group elements (big pubkeys).
- `messages` (uint256[2][]): An array of messages signed to produce the signature.

#### Usage

To verify multiple non-aggregated signatures, call the verifyMultiple() function with the required parameters:

```solidity
(bool checkResult, bool callSuccess) = IBLS.instance.verifyMultiple(signature, pubkeys, messages);
```

### verifyMultipleSameMsg()

This function verifies an aggregated signature where the same message is signed.

#### Parameters

- `signature` (uint256[2]): A 64-byte G1 group element (small signature).
- `pubkeys` (uint256[4][]): An array of 128-byte G2 group elements (big pubkeys).
- `message` (uint256[2]): The message signed by all to produce the signature.

#### Usage

To verify an aggregated signature with the same message, call the verifyMultipleSameMsg() function with the required parameters:

```solidity
(bool checkResult, bool callSuccess) = IBLS.instance.verifyMultipleSameMsg(signature, pubkeys, message);
```

### mapToPoint()

This function maps a field element to the curve.

#### Parameters

`_x`(uint256): A valid field element.

#### Usage

To map a field element to the curve, call the mapToPoint() function with the required parameter:

```solidity
uint256[2] memory point = IBLS.instance.mapToPoint(_x);
```

### isValidSignature()

This function checks if a signature is formatted correctly and valid. It will revert if improperly formatted and return false if invalid.

#### Parameters

`signature` (uint256[2]): The BLS signature.

#### Usage

To check if a signature is valid, call the isValidSignature() function with the required parameter:

```solidity
bool isValid = IBLS.instance.isValidSignature(signature);
```

### isOnCurveG1()

This function checks if a point in the finite field Fq (x, y) is on the G1 curve.

#### Parameters

`point` (uint256[2]): An array with x and y values of the point.

#### Usage

To check if a point is on the G1 curve, call the isOnCurveG1() function with the required parameter:

```solidity
bool isOnCurve = IBLS.instance.isOnCurveG1(point);
```

### isOnCurveG2()

This function checks if a point in the finite field Fq (x, y) is on the G2 curve.

#### Parameters
