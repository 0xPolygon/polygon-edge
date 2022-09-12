// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/core/protocol.
package protocol

import "github.com/libp2p/go-libp2p/core/protocol"

// ID is an identifier used to write protocol headers in streams.
// Deprecated: use github.com/libp2p/go-libp2p/core/protocol.ID instead
type ID = protocol.ID

// These are reserved protocol.IDs.
const (
	// Deprecated: use github.com/libp2p/go-libp2p/core/protocol.TestingID instead
	TestingID ID = protocol.TestingID
)

// ConvertFromStrings is a convenience function that takes a slice of strings and
// converts it to a slice of protocol.ID.
// Deprecated: use github.com/libp2p/go-libp2p/core/protocol.ConvertFromStrings instead
func ConvertFromStrings(ids []string) (res []ID) {
	return protocol.ConvertFromStrings(ids)
}

// ConvertToStrings is a convenience function that takes a slice of protocol.ID and
// converts it to a slice of strings.
// Deprecated: use github.com/libp2p/go-libp2p/core/protocol.ConvertToStrings instead
func ConvertToStrings(ids []ID) (res []string) {
	return protocol.ConvertToStrings(ids)
}
