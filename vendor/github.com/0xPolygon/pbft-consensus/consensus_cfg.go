package pbft

import (
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/0xPolygon/pbft-consensus/stats"
)

const (
	defaultTimeout     = 2 * time.Second
	maxTimeout         = 300 * time.Second
	maxTimeoutExponent = 8
)

type RoundTimeout func(round uint64) <-chan time.Time

type StatsCallback func(stats.Stats)

type ConfigOption func(*Config)

func WithLogger(l Logger) ConfigOption {
	return func(c *Config) {
		c.Logger = l
	}
}

func WithTracer(t trace.Tracer) ConfigOption {
	return func(c *Config) {
		c.Tracer = t
	}
}

func WithRoundTimeout(roundTimeout RoundTimeout) ConfigOption {
	return func(c *Config) {
		if roundTimeout != nil {
			c.RoundTimeout = roundTimeout
		}
	}
}

func WithNotifier(notifier StateNotifier) ConfigOption {
	return func(c *Config) {
		if notifier != nil {
			c.Notifier = notifier
		}
	}
}

func WithVotingPower(vp map[NodeID]uint64) ConfigOption {
	return func(c *Config) {
		if len(vp) > 0 {
			c.VotingPower = vp
		}
	}
}

type Config struct {
	// ProposalTimeout is the time to wait for the proposal
	// from the validator. It defaults to Timeout
	ProposalTimeout time.Duration

	// Timeout is the time to wait for validation and
	// round change messages
	Timeout time.Duration

	// Logger is the logger to output info
	Logger Logger

	// Tracer is the OpenTelemetry tracer to log traces
	Tracer trace.Tracer

	// RoundTimeout is a function that calculates timeout based on a round number
	RoundTimeout RoundTimeout

	// Notifier is a reference to the struct which encapsulates handling messages and timeouts
	Notifier StateNotifier

	StatsCallback StatsCallback

	VotingPower map[NodeID]uint64
}

func DefaultConfig() *Config {
	return &Config{
		Timeout:         defaultTimeout,
		ProposalTimeout: defaultTimeout,
		Logger:          log.New(os.Stderr, "", log.LstdFlags),
		Tracer:          trace.NewNoopTracerProvider().Tracer(""),
		RoundTimeout:    exponentialTimeout,
		Notifier:        &DefaultStateNotifier{},
	}
}

func (c *Config) ApplyOps(opts ...ConfigOption) {
	for _, opt := range opts {
		opt(c)
	}
}

// exponentialTimeout is the default RoundTimeout function
func exponentialTimeout(round uint64) <-chan time.Time {
	return time.NewTimer(exponentialTimeoutDuration(round)).C
}

// --- package-level helper functions ---
// exponentialTimeout calculates the timeout duration depending on the current round.
// Round acts as an exponent when determining timeout (2^round).
func exponentialTimeoutDuration(round uint64) time.Duration {
	timeout := defaultTimeout
	// limit exponent to be in range of maxTimeout (<=8) otherwise use maxTimeout
	// this prevents calculating timeout that is greater than maxTimeout and
	// possible overflow for calculating timeout for rounds >33 since duration is in nanoseconds stored in int64
	if round <= maxTimeoutExponent {
		timeout += time.Duration(1<<round) * time.Second
	} else {
		timeout = maxTimeout
	}

	return timeout
}

// DefaultStateNotifier is a null object implementation of StateNotifier interface
type DefaultStateNotifier struct {
}

// HandleTimeout implements StateNotifier interface
func (d *DefaultStateNotifier) HandleTimeout(NodeID, MsgType, *View) {}

// ReadNextMessage is an implementation of StateNotifier interface
func (d *DefaultStateNotifier) ReadNextMessage(p *Pbft) (*MessageReq, []*MessageReq) {
	return p.ReadMessageWithDiscards()
}
