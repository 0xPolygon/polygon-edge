package tracker

import "github.com/umbracle/go-web3/abi"

/* ABI events as defined by StateSender.sol */

var (
	NewRegistrationEvent = abi.MustNewEvent(`event NewRegistration(
	address indexed user,
	address indexed sender,
    address indexed receiver)`)

	RegistrationUpdatedEvent = abi.MustNewEvent(`event RegistrationUpdated(
	address indexed user,
	address indexed sender,
	address indexed receiver)`)

	StateSyncedEvent = abi.MustNewEvent(`event StateSynced(
	uint256 indexed id,
	address indexed contractAddress,
	bytes data)`)

	PoCEvent = abi.MustNewEvent(`event MyEvent(
        address indexed sender)`)

	AnotherEvent = abi.MustNewEvent(`event AnotherEvent(
        address indexed sender)`)
)
