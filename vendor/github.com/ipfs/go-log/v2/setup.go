package log

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func init() {
	SetupLogging()
}

// Logging environment variables
const (
	// IPFS_* prefixed env vars kept for backwards compatibility
	// for this release. They will not be available in the next
	// release.
	//
	// GOLOG_* env vars take precedences over IPFS_* env vars.
	envIPFSLogging    = "IPFS_LOGGING"
	envIPFSLoggingFmt = "IPFS_LOGGING_FMT"

	envLogging    = "GOLOG_LOG_LEVEL"
	envLoggingFmt = "GOLOG_LOG_FMT"

	envLoggingFile = "GOLOG_FILE" // /path/to/file
)

// ErrNoSuchLogger is returned when the util pkg is asked for a non existant logger
var ErrNoSuchLogger = errors.New("Error: No such logger")

// loggers is the set of loggers in the system
var loggerMutex sync.RWMutex
var loggers = make(map[string]*zap.SugaredLogger)
var levels = make(map[string]zap.AtomicLevel)

// SetupLogging will initialize the logger backend and set the flags.
// TODO calling this in `init` pushes all configuration to env variables
// - move it out of `init`? then we need to change all the code (js-ipfs, go-ipfs) to call this explicitly
// - have it look for a config file? need to define what that is
var zapCfg = zap.NewProductionConfig()

func SetupLogging() {
	loggingFmt := os.Getenv(envLoggingFmt)
	if loggingFmt == "" {
		loggingFmt = os.Getenv(envIPFSLoggingFmt)
	}
	// colorful or plain
	switch loggingFmt {
	case "nocolor":
		zapCfg.Encoding = "console"
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	case "json":
		zapCfg.Encoding = "json"
	default:
		zapCfg.Encoding = "console"
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	zapCfg.Sampling = nil
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zapCfg.OutputPaths = []string{"stderr"}
	// check if we log to a file
	if logfp := os.Getenv(envLoggingFile); len(logfp) > 0 {
		if path, err := normalizePath(logfp); err != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve log path '%q', logging to stderr only: %s\n", logfp, err)
		} else {
			zapCfg.OutputPaths = append(zapCfg.OutputPaths, path)
		}
	}

	// set the backend(s)
	lvl := LevelError

	logenv := os.Getenv(envLogging)
	if logenv == "" {
		logenv = os.Getenv(envIPFSLogging)
	}

	if logenv != "" {
		var err error
		lvl, err = LevelFromString(logenv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error setting log levels: %s\n", err)
		}
	}
	zapCfg.Level.SetLevel(zapcore.Level(lvl))

	SetAllLoggers(lvl)
}

// SetDebugLogging calls SetAllLoggers with logging.DEBUG
func SetDebugLogging() {
	SetAllLoggers(LevelDebug)
}

// SetAllLoggers changes the logging level of all loggers to lvl
func SetAllLoggers(lvl LogLevel) {
	loggerMutex.RLock()
	defer loggerMutex.RUnlock()

	for _, l := range levels {
		l.SetLevel(zapcore.Level(lvl))
	}
}

// SetLogLevel changes the log level of a specific subsystem
// name=="*" changes all subsystems
func SetLogLevel(name, level string) error {
	lvl, err := LevelFromString(level)
	if err != nil {
		return err
	}

	// wildcard, change all
	if name == "*" {
		SetAllLoggers(lvl)
		return nil
	}

	loggerMutex.RLock()
	defer loggerMutex.RUnlock()

	// Check if we have a logger by that name
	if _, ok := levels[name]; !ok {
		return ErrNoSuchLogger
	}

	levels[name].SetLevel(zapcore.Level(lvl))

	return nil
}

// SetLogLevelRegex sets all loggers to level `l` that match expression `e`.
// An error is returned if `e` fails to compile.
func SetLogLevelRegex(e, l string) error {
	lvl, err := LevelFromString(l)
	if err != nil {
		return err
	}

	rem, err := regexp.Compile(e)
	if err != nil {
		return err
	}

	loggerMutex.Lock()
	defer loggerMutex.Unlock()
	for name := range loggers {
		if rem.MatchString(name) {
			levels[name].SetLevel(zapcore.Level(lvl))
		}
	}
	return nil
}

// GetSubsystems returns a slice containing the
// names of the current loggers
func GetSubsystems() []string {
	loggerMutex.RLock()
	defer loggerMutex.RUnlock()
	subs := make([]string, 0, len(loggers))

	for k := range loggers {
		subs = append(subs, k)
	}
	return subs
}

func getLogger(name string) *zap.SugaredLogger {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()
	log, ok := loggers[name]
	if !ok {
		levels[name] = zap.NewAtomicLevelAt(zapCfg.Level.Level())
		cfg := zap.Config(zapCfg)
		cfg.Level = levels[name]
		newlog, err := cfg.Build()
		if err != nil {
			panic(err)
		}
		log = newlog.Named(name).Sugar()
		loggers[name] = log
	}

	return log
}
