package core

import "errors"

var (
	// errInconsistentSubject is returned when received subject is different from
	// current subject.
	errInconsistentSubject = errors.New("inconsistent subjects")
	// errNotFromProposer is returned when received message is supposed to be from
	// proposer.
	errNotFromProposer = errors.New("message does not come from proposer")
	// errIgnored is returned when a message was ignored.
	errIgnored = errors.New("message is ignored")
	// errFutureMessage is returned when current view is earlier than the
	// view of the received message.
	errFutureMessage = errors.New("future message")
	// errOldMessage is returned when the received message's view is earlier
	// than current view.
	errOldMessage = errors.New("old message")
	// errInvalidMessage is returned when the message is malformed.
	errInvalidMessage = errors.New("invalid message")
	// errFailedDecodePreprepare is returned when the PRE-PREPARE message is malformed.
	errFailedDecodePreprepare = errors.New("failed to decode PRE-PREPARE")
	// errFailedDecodePrepare is returned when the PREPARE message is malformed.
	errFailedDecodePrepare = errors.New("failed to decode PREPARE")
	// errFailedDecodeCommit is returned when the COMMIT message is malformed.
	errFailedDecodeCommit = errors.New("failed to decode COMMIT")
	// errFailedDecodeMessageSet is returned when the message set is malformed.
	errFailedDecodeMessageSet = errors.New("failed to decode message set")
)
