package record

import (
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p/core/record"
)

// Envelope contains an arbitrary []byte payload, signed by a libp2p peer.
//
// Envelopes are signed in the context of a particular "domain", which is a
// string specified when creating and verifying the envelope. You must know the
// domain string used to produce the envelope in order to verify the signature
// and access the payload.
// Deprecated: use github.com/libp2p/go-libp2p/core/record.Envelope instead
type Envelope = record.Envelope

// Deprecated: use github.com/libp2p/go-libp2p/core/record.ErrEmptyDomain instead
var ErrEmptyDomain = record.ErrEmptyDomain

// Deprecated: use github.com/libp2p/go-libp2p/core/record.ErrEmptyPayloadType instead
var ErrEmptyPayloadType = record.ErrEmptyPayloadType

// Deprecated: use github.com/libp2p/go-libp2p/core/record.ErrInvalidSignature instead
var ErrInvalidSignature = record.ErrInvalidSignature

// Seal marshals the given Record, places the marshaled bytes inside an Envelope,
// and signs with the given private key.
// Deprecated: use github.com/libp2p/go-libp2p/core/record.Seal instead
func Seal(rec Record, privateKey crypto.PrivKey) (*Envelope, error) {
	return record.Seal(rec, privateKey)
}

// ConsumeEnvelope unmarshals a serialized Envelope and validates its
// signature using the provided 'domain' string. If validation fails, an error
// is returned, along with the unmarshalled envelope so it can be inspected.
//
// On success, ConsumeEnvelope returns the Envelope itself, as well as the inner payload,
// unmarshalled into a concrete Record type. The actual type of the returned Record depends
// on what has been registered for the Envelope's PayloadType (see RegisterType for details).
//
// You can type assert on the returned Record to convert it to an instance of the concrete
// Record type:
//
//	envelope, rec, err := ConsumeEnvelope(envelopeBytes, peer.PeerRecordEnvelopeDomain)
//	if err != nil {
//	  handleError(envelope, err)  // envelope may be non-nil, even if errors occur!
//	  return
//	}
//	peerRec, ok := rec.(*peer.PeerRecord)
//	if ok {
//	  doSomethingWithPeerRecord(peerRec)
//	}
//
// Important: you MUST check the error value before using the returned Envelope. In some error
// cases, including when the envelope signature is invalid, both the Envelope and an error will
// be returned. This allows you to inspect the unmarshalled but invalid Envelope. As a result,
// you must not assume that any non-nil Envelope returned from this function is valid.
//
// If the Envelope signature is valid, but no Record type is registered for the Envelope's
// PayloadType, ErrPayloadTypeNotRegistered will be returned, along with the Envelope and
// a nil Record.
// Deprecated: use github.com/libp2p/go-libp2p/core/record.ConsumeEnvelope instead
func ConsumeEnvelope(data []byte, domain string) (envelope *Envelope, rec Record, err error) {
	return record.ConsumeEnvelope(data, domain)
}

// ConsumeTypedEnvelope unmarshals a serialized Envelope and validates its
// signature. If validation fails, an error is returned, along with the unmarshalled
// envelope so it can be inspected.
//
// Unlike ConsumeEnvelope, ConsumeTypedEnvelope does not try to automatically determine
// the type of Record to unmarshal the Envelope's payload into. Instead, the caller provides
// a destination Record instance, which will unmarshal the Envelope payload. It is the caller's
// responsibility to determine whether the given Record type is able to unmarshal the payload
// correctly.
//
//	rec := &MyRecordType{}
//	envelope, err := ConsumeTypedEnvelope(envelopeBytes, rec)
//	if err != nil {
//	  handleError(envelope, err)
//	}
//	doSomethingWithRecord(rec)
//
// Important: you MUST check the error value before using the returned Envelope. In some error
// cases, including when the envelope signature is invalid, both the Envelope and an error will
// be returned. This allows you to inspect the unmarshalled but invalid Envelope. As a result,
// you must not assume that any non-nil Envelope returned from this function is valid.
// Deprecated: use github.com/libp2p/go-libp2p/core/record.ConsumeTypedEnvelope instead
func ConsumeTypedEnvelope(data []byte, destRecord Record) (envelope *Envelope, err error) {
	return record.ConsumeTypedEnvelope(data, destRecord)
}

// UnmarshalEnvelope unmarshals a serialized Envelope protobuf message,
// without validating its contents. Most users should use ConsumeEnvelope.
// Deprecated: use github.com/libp2p/go-libp2p/core/record.UnmarshalEnvelope instead
func UnmarshalEnvelope(data []byte) (*Envelope, error) {
	return record.UnmarshalEnvelope(data)
}
