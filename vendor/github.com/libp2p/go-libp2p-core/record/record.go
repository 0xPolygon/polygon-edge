// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/core/record.
package record

import (
	"github.com/libp2p/go-libp2p/core/record"
)

var (
	// ErrPayloadTypeNotRegistered is returned from ConsumeEnvelope when the Envelope's
	// PayloadType does not match any registered Record types.
	// Deprecated: use github.com/libp2p/go-libp2p/core/record.ErrPayloadTypeNotRegistered instead
	ErrPayloadTypeNotRegistered = record.ErrPayloadTypeNotRegistered
)

// Record represents a data type that can be used as the payload of an Envelope.
// The Record interface defines the methods used to marshal and unmarshal a Record
// type to a byte slice.
//
// Record types may be "registered" as the default for a given Envelope.PayloadType
// using the RegisterType function. Once a Record type has been registered,
// an instance of that type will be created and used to unmarshal the payload of
// any Envelope with the registered PayloadType when the Envelope is opened using
// the ConsumeEnvelope function.
//
// To use an unregistered Record type instead, use ConsumeTypedEnvelope and pass in
// an instance of the Record type that you'd like the Envelope's payload to be
// unmarshaled into.
// Deprecated: use github.com/libp2p/go-libp2p/core/record.Record instead
type Record = record.Record

// RegisterType associates a binary payload type identifier with a concrete
// Record type. This is used to automatically unmarshal Record payloads from Envelopes
// when using ConsumeEnvelope, and to automatically marshal Records and determine the
// correct PayloadType when calling Seal.
//
// Callers must provide an instance of the record type to be registered, which must be
// a pointer type. Registration should be done in the init function of the package
// where the Record type is defined:
//
//	package hello_record
//	import record "github.com/libp2p/go-libp2p-core/record"
//
//	func init() {
//	    record.RegisterType(&HelloRecord{})
//	}
//
//	type HelloRecord struct { } // etc..
//
// Deprecated: use github.com/libp2p/go-libp2p/core/record.RegisterType instead
func RegisterType(prototype Record) {
	record.RegisterType(prototype)
}
