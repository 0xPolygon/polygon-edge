package rootchain

// Config defines the rootchain config mapping "0xa... (rootchain address)" => []EventConfig
type Config struct {
	RootchainAddresses []EventConfig `json:"rootchainAddresses"`
}

// EventConfig defines a single event that needs to be listened for on the rootchain,
// and corresponding local metadata
type EventConfig struct {
	RootchainURL       string      `json:"rootchainURL"`
	EventABI           string      `json:"eventABI"`
	MethodABI          string      `json:"methodABI"`
	MethodName         string      `json:"methodName"`
	LocalAddress       string      `json:"localAddress"`
	PayloadType        PayloadType `json:"payloadType"`
	BlockConfirmations uint64      `json:"blockConfirmations"`
}
