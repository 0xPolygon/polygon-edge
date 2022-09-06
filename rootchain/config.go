package rootchain

// Config defines the rootchain config mapping "0xa... (rootchain address)" => []ConfigEvent
type Config struct {
	RootchainAddresses map[string][]ConfigEvent `json:"RootchainAddresses"`
}

// ConfigEvent defines a single event that needs to be listened for on the rootchain,
// and corresponding local metadata
type ConfigEvent struct {
	EventABI     string      `json:"eventABI"`
	MethodABI    string      `json:"methodABI"`
	LocalAddress string      `json:"localAddress"`
	PayloadType  PayloadType `json:"PayloadType"`
}
