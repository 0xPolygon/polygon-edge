// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package profiler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gopkg.in/DataDog/dd-trace-go.v1/internal"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/globalconfig"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/osinfo"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/version"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler/internal/extensions"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler/internal/immutable"

	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// DefaultMutexFraction specifies the mutex profile fraction to be used with the mutex profiler.
	// For more information or for changing this value, check MutexProfileFraction
	DefaultMutexFraction = 10

	// DefaultBlockRate specifies the default block profiling rate (in ns) used
	// by the block profiler. For more information or for changing this value,
	// check BlockProfileRate(). The default value of 100ms is somewhat
	// arbitrary. There is no provably safe value that will guarantee low
	// overhead for this profile type for all workloads. We don't recommend
	// enabling it under normal circumstances. See the link below for more
	// information: https://github.com/DataDog/go-profiler-notes/pull/15/files
	DefaultBlockRate = 100000000

	// DefaultPeriod specifies the default period at which profiles will be collected.
	DefaultPeriod = time.Minute

	// DefaultDuration specifies the default length of the CPU profile snapshot.
	DefaultDuration = time.Minute

	// DefaultUploadTimeout specifies the default timeout for uploading profiles.
	// It can be overwritten using the DD_PROFILING_UPLOAD_TIMEOUT env variable
	// or the WithUploadTimeout option.
	DefaultUploadTimeout = 10 * time.Second
)

const (
	defaultAPIURL    = "https://intake.profile.datadoghq.com/v1/input"
	defaultAgentHost = "localhost"
	defaultAgentPort = "8126"
	defaultEnv       = "none"
)

var defaultClient = &http.Client{
	// We copy the transport to avoid using the default one, as it might be
	// augmented with tracing and we don't want these calls to be recorded.
	// See https://golang.org/pkg/net/http/#DefaultTransport .
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

var defaultProfileTypes = []ProfileType{MetricsProfile, CPUProfile, HeapProfile}

type config struct {
	apiKey    string
	agentless bool
	// targetURL is the upload destination URL. It will be set by the profiler on start to either apiURL or agentURL
	// based on the other options.
	targetURL         string
	apiURL            string // apiURL is the Datadog intake API URL
	agentURL          string // agentURL is the Datadog agent profiling URL
	service, env      string
	hostname          string
	statsd            StatsdClient
	httpClient        *http.Client
	tags              immutable.StringSlice
	types             map[ProfileType]struct{}
	period            time.Duration
	cpuDuration       time.Duration
	cpuProfileRate    int
	uploadTimeout     time.Duration
	maxGoroutinesWait int
	mutexFraction     int
	blockRate         int
	outputDir         string
	deltaProfiles     bool
	logStartup        bool
	cmemprofEnabled   bool
	cmemprofRate      int
}

// logStartup records the configuration to the configured logger in JSON format
func logStartup(c *config) {
	info := struct {
		Date                 string   `json:"date"`         // ISO 8601 date and time of start
		OSName               string   `json:"os_name"`      // Windows, Darwin, Debian, etc.
		OSVersion            string   `json:"os_version"`   // Version of the OS
		Version              string   `json:"version"`      // Profiler version
		Lang                 string   `json:"lang"`         // "Go"
		LangVersion          string   `json:"lang_version"` // Go version, e.g. go1.18
		Hostname             string   `json:"hostname"`
		DeltaProfiles        bool     `json:"delta_profiles"`
		Service              string   `json:"service"`
		Env                  string   `json:"env"`
		TargetURL            string   `json:"target_url"`
		Agentless            bool     `json:"agentless"`
		Tags                 []string `json:"tags"`
		ProfilePeriod        string   `json:"profile_period"`
		EnabledProfiles      []string `json:"enabled_profiles"`
		CPUDuration          string   `json:"cpu_duration"`
		CPUProfileRate       int      `json:"cpu_profile_rate"`
		BlockProfileRate     int      `json:"block_profile_rate"`
		MutexProfileFraction int      `json:"mutex_profile_fraction"`
		MaxGoroutinesWait    int      `json:"max_goroutines_wait"`
		UploadTimeout        string   `json:"upload_timeout"`
		CmemprofEnabled      bool     `json:"cmemprof_enabled"`
		CmemprofRate         int      `json:"cmemprof_rate"`
	}{
		Date:                 time.Now().Format(time.RFC3339),
		OSName:               osinfo.OSName(),
		OSVersion:            osinfo.OSVersion(),
		Version:              version.Tag,
		Lang:                 "Go",
		LangVersion:          runtime.Version(),
		Hostname:             c.hostname,
		DeltaProfiles:        c.deltaProfiles,
		Service:              c.service,
		Env:                  c.env,
		TargetURL:            c.targetURL,
		Agentless:            c.agentless,
		Tags:                 c.tags.Slice(),
		ProfilePeriod:        c.period.String(),
		CPUDuration:          c.cpuDuration.String(),
		CPUProfileRate:       c.cpuProfileRate,
		BlockProfileRate:     c.blockRate,
		MutexProfileFraction: c.mutexFraction,
		MaxGoroutinesWait:    c.maxGoroutinesWait,
		UploadTimeout:        c.uploadTimeout.String(),
		CmemprofEnabled:      c.cmemprofEnabled,
		CmemprofRate:         c.cmemprofRate,
	}
	for t := range c.types {
		info.EnabledProfiles = append(info.EnabledProfiles, t.String())
	}
	b, err := json.Marshal(info)
	if err != nil {
		log.Error("Marshaling profiler configuration: %s", err)
		return
	}
	log.Info("Profiler configuration: %s\n", b)
}

func urlForSite(site string) (string, error) {
	u := fmt.Sprintf("https://intake.profile.%s/v1/input", site)
	_, err := url.Parse(u)
	return u, err
}

// isAPIKeyValid reports whether the given string is a structurally valid API key
func isAPIKeyValid(key string) bool {
	if len(key) != 32 {
		return false
	}
	for _, c := range key {
		if c > unicode.MaxASCII || (!unicode.IsLower(c) && !unicode.IsNumber(c)) {
			return false
		}
	}
	return true
}

func (c *config) addProfileType(t ProfileType) {
	if c.types == nil {
		c.types = make(map[ProfileType]struct{})
	}
	c.types[t] = struct{}{}
}

func defaultConfig() (*config, error) {
	c := config{
		env:               defaultEnv,
		apiURL:            defaultAPIURL,
		service:           filepath.Base(os.Args[0]),
		statsd:            &statsd.NoOpClient{},
		httpClient:        defaultClient,
		period:            DefaultPeriod,
		cpuDuration:       DefaultDuration,
		blockRate:         DefaultBlockRate,
		mutexFraction:     DefaultMutexFraction,
		uploadTimeout:     DefaultUploadTimeout,
		maxGoroutinesWait: 1000, // arbitrary value, should limit STW to ~30ms
		deltaProfiles:     internal.BoolEnv("DD_PROFILING_DELTA", true),
		logStartup:        internal.BoolEnv("DD_TRACE_STARTUP_LOGS", true),
		cmemprofEnabled:   false,
		cmemprofRate:      extensions.DefaultCAllocationSamplingRate,
	}
	c.tags = c.tags.Append(fmt.Sprintf("process_id:%d", os.Getpid()))
	for _, t := range defaultProfileTypes {
		c.addProfileType(t)
	}

	agentHost, agentPort := defaultAgentHost, defaultAgentPort
	if v := os.Getenv("DD_AGENT_HOST"); v != "" {
		agentHost = v
	}
	if v := os.Getenv("DD_TRACE_AGENT_PORT"); v != "" {
		agentPort = v
	}
	WithAgentAddr(net.JoinHostPort(agentHost, agentPort))(&c)
	if v := os.Getenv("DD_PROFILING_UPLOAD_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("DD_PROFILING_UPLOAD_TIMEOUT: %s", err)
		}
		WithUploadTimeout(d)(&c)
	}
	if v := os.Getenv("DD_API_KEY"); v != "" {
		WithAPIKey(v)(&c)
	}
	if internal.BoolEnv("DD_PROFILING_AGENTLESS", false) {
		WithAgentlessUpload()(&c)
	}
	if v := os.Getenv("DD_SITE"); v != "" {
		WithSite(v)(&c)
	}
	if v := os.Getenv("DD_ENV"); v != "" {
		WithEnv(v)(&c)
	}
	if v := os.Getenv("DD_SERVICE"); v != "" {
		WithService(v)(&c)
	}
	if v := os.Getenv("DD_VERSION"); v != "" {
		WithVersion(v)(&c)
	}
	if v := os.Getenv("DD_TAGS"); v != "" {
		sep := " "
		if strings.Index(v, ",") > -1 {
			// falling back to comma as separator
			sep = ","
		}
		for _, tag := range strings.Split(v, sep) {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			WithTags(tag)(&c)
		}
	}
	WithTags(
		"profiler_version:"+version.Tag,
		"runtime_version:"+strings.TrimPrefix(runtime.Version(), "go"),
		"runtime_compiler:"+runtime.Compiler,
		"runtime_arch:"+runtime.GOARCH,
		"runtime_os:"+runtime.GOOS,
		"runtime-id:"+globalconfig.RuntimeID(),
	)(&c)
	// not for public use
	if v := os.Getenv("DD_PROFILING_URL"); v != "" {
		WithURL(v)(&c)
	}
	// not for public use
	if v := os.Getenv("DD_PROFILING_OUTPUT_DIR"); v != "" {
		withOutputDir(v)(&c)
	}
	if v := os.Getenv("DD_PROFILING_WAIT_PROFILE_MAX_GOROUTINES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("DD_PROFILING_WAIT_PROFILE_MAX_GOROUTINES: %s", err)
		}
		c.maxGoroutinesWait = n
	}
	c.cmemprofEnabled = internal.BoolEnv("DD_PROFILING_CMEMPROF_ENABLED", false)
	if v := os.Getenv("DD_PROFILING_CMEMPROF_SAMPLING_RATE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("DD_PROFILING_CMEMPROF_SAMPLING_RATE: %s", err)
		}
		c.cmemprofRate = n
	}
	return &c, nil
}

// An Option is used to configure the profiler's behaviour.
type Option func(*config)

// WithAgentAddr specifies the address to use when reaching the Datadog Agent.
func WithAgentAddr(hostport string) Option {
	return func(cfg *config) {
		cfg.agentURL = "http://" + hostport + "/profiling/v1/input"
	}
}

// WithAPIKey sets the Datadog API Key and takes precedence over the DD_API_KEY
// env variable. Historically this option was used to enable agentless
// uploading, but as of dd-trace-go v1.30.0 the behavior has changed to always
// default to agent based uploading which doesn't require an API key. So if you
// currently don't have an agent running on the default localhost:8126 hostport
// you need to set it up, or use WithAgentAddr to specify the hostport location
// of the agent. See WithAgentlessUpload for more information.
func WithAPIKey(key string) Option {
	return func(cfg *config) {
		cfg.apiKey = key
	}
}

// WithAgentlessUpload is currently for internal usage only and not officially
// supported. You should not enable it unless somebody at Datadog instructed
// you to do so. It allows to skip the agent and talk to the Datadog API
// directly using the provided API key.
func WithAgentlessUpload() Option {
	return func(cfg *config) {
		cfg.agentless = true
	}
}

// WithDeltaProfiles specifies if delta profiles are enabled. The default value
// is true. This option takes precedence over the DD_PROFILING_DELTA
// environment variable that can be set to "true" or "false" as well. See
// https://dtdg.co/go-delta-profile-docs for more information.
func WithDeltaProfiles(enabled bool) Option {
	return func(cfg *config) {
		cfg.deltaProfiles = enabled
	}
}

// WithURL specifies the HTTP URL for the Datadog Profiling API.
func WithURL(url string) Option {
	return func(cfg *config) {
		cfg.apiURL = url
	}
}

// WithPeriod specifies the interval at which to collect profiles.
func WithPeriod(d time.Duration) Option {
	return func(cfg *config) {
		cfg.period = d
	}
}

// CPUDuration specifies the length at which to collect CPU profiles.
func CPUDuration(d time.Duration) Option {
	return func(cfg *config) {
		cfg.cpuDuration = d
	}
}

// CPUProfileRate sets the sampling frequency for CPU profiling. A sample will
// be taken once for every (1 / hz) seconds of on-CPU time. If not given,
// profiling will use the default rate from the runtime/pprof.StartCPUProfile
// function, which is 100 as of Go 1.0.
//
// Setting a different profile rate will result in a spurious warning every time
// CPU profling is started, like "cannot set cpu profile rate until previous
// profile has finished". This is a known issue, but the rate will still be set
// correctly and CPU profiling will work.
func CPUProfileRate(hz int) Option {
	return func(cfg *config) {
		cfg.cpuProfileRate = hz
	}
}

// MutexProfileFraction turns on mutex profiles with rate indicating the fraction
// of mutex contention events reported in the mutex profile.
// On average, 1/rate events are reported.
// Setting an aggressive rate can hurt performance.
// For more information on this value, check runtime.SetMutexProfileFraction.
func MutexProfileFraction(rate int) Option {
	return func(cfg *config) {
		cfg.addProfileType(MutexProfile)
		cfg.mutexFraction = rate
	}
}

// BlockProfileRate turns on block profiles with the given rate. We do not
// recommend enabling this profile type, see DefaultBlockRate for more
// information. The rate is given in nanoseconds and a block event with a given
// duration has a min(duration/rate, 1) chance of getting sampled.
func BlockProfileRate(rate int) Option {
	return func(cfg *config) {
		cfg.addProfileType(BlockProfile)
		cfg.blockRate = rate
	}
}

// WithProfileTypes specifies the profile types to be collected by the profiler.
func WithProfileTypes(types ...ProfileType) Option {
	return func(cfg *config) {
		// reset the types and only use what the user has specified
		for k := range cfg.types {
			delete(cfg.types, k)
		}
		cfg.addProfileType(MetricsProfile) // always report metrics
		for _, t := range types {
			cfg.addProfileType(t)
		}
	}
}

// WithService specifies the service name to attach to a profile.
func WithService(name string) Option {
	return func(cfg *config) {
		cfg.service = name
	}
}

// WithEnv specifies the environment to which these profiles should be registered.
func WithEnv(env string) Option {
	return func(cfg *config) {
		cfg.env = env
	}
}

// WithVersion specifies the service version tag to attach to profiles
func WithVersion(version string) Option {
	return WithTags("version:" + version)
}

// WithTags specifies a set of tags to be attached to the profiler. These may help
// filter the profiling view based on various information.
func WithTags(tags ...string) Option {
	return func(cfg *config) {
		cfg.tags = cfg.tags.Append(tags...)
	}
}

// WithStatsd specifies an optional statsd client to use for metrics. By default,
// no metrics are sent.
func WithStatsd(client StatsdClient) Option {
	return func(cfg *config) {
		cfg.statsd = client
	}
}

// WithUploadTimeout specifies the timeout to use for uploading profiles. The
// default timeout is specified by DefaultUploadTimeout or the
// DD_PROFILING_UPLOAD_TIMEOUT env variable. Using a negative value or 0 will
// cause an error when starting the profiler.
func WithUploadTimeout(d time.Duration) Option {
	return func(cfg *config) {
		cfg.uploadTimeout = d
	}
}

// WithSite specifies the datadog site (datadoghq.com, datadoghq.eu, etc.)
// which profiles will be sent to.
func WithSite(site string) Option {
	return func(cfg *config) {
		u, err := urlForSite(site)
		if err != nil {
			log.Error("profiler: invalid site provided, using %s (%s)", defaultAPIURL, err)
			return
		}
		cfg.apiURL = u
	}
}

// WithHTTPClient specifies the HTTP client to use when submitting profiles to Site.
// In general, using this method is only necessary if you have need to customize the
// transport layer, for instance when using a unix domain socket.
func WithHTTPClient(client *http.Client) Option {
	return func(cfg *config) {
		cfg.httpClient = client
	}
}

// WithUDS configures the HTTP client to dial the Datadog Agent via the specified Unix Domain Socket path.
func WithUDS(socketPath string) Option {
	return WithHTTPClient(&http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	})
}

// withOutputDir writes a copy of all uploaded profiles to the given
// directory. This is intended for local development or debugging uploading
// issues. The directory will keep growing, no cleanup is performed.
func withOutputDir(dir string) Option {
	return func(cfg *config) {
		cfg.outputDir = dir
	}
}

// WithLogStartup toggles logging the configuration of the profiler to standard
// error when profiling is started. The configuration is logged in a JSON
// format. This option is enabled by default.
func WithLogStartup(enabled bool) Option {
	return func(cfg *config) {
		cfg.logStartup = enabled
	}
}
