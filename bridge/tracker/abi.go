package tracker

import "github.com/umbracle/go-web3/abi"

/* ABI events as defined by StateSender.sol */

var NewRegistrationEvent = abi.MustNewEvent(`NewRegistration(
	address indexed user,
	address indexed sender,
    address indexed receiver
)`)

var RegistrationUpdatedEvent = abi.MustNewEvent(`RegistrationUpdated(
	address indexed user,
	address indexed sender,
	address indexed receiver
)`)

var StateSyncedEvent = abi.MustNewEvent(`event StateSynced(
	uint256 indexed id,
	address indexed contractAddress,
	bytes data
)`)
