// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package profiler

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/internal"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/telemetry"

	pprofile "github.com/google/pprof/profile"
)

// outChannelSize specifies the size of the profile output channel.
const outChannelSize = 5

var (
	mu             sync.Mutex
	activeProfiler *profiler
	containerID    = internal.ContainerID() // replaced in tests
)

// Start starts the profiler. It may return an error if an API key is not provided by means of
// the WithAPIKey option, or if a hostname is not found.
func Start(opts ...Option) error {
	mu.Lock()
	defer mu.Unlock()
	if activeProfiler != nil {
		activeProfiler.stop()
	}
	p, err := newProfiler(opts...)
	if err != nil {
		return err
	}
	activeProfiler = p
	activeProfiler.run()
	return nil
}

// Stop cancels any ongoing profiling or upload operations and returns after
// everything has been stopped.
func Stop() {
	mu.Lock()
	if activeProfiler != nil {
		activeProfiler.stop()
		activeProfiler = nil
	}
	mu.Unlock()
}

// profiler collects and sends preset profiles to the Datadog API at a given frequency
// using a given configuration.
type profiler struct {
	mu         sync.Mutex
	cfg        *config                      // profile configuration
	out        chan batch                   // upload queue
	uploadFunc func(batch) error            // defaults to (*profiler).upload; replaced in tests
	exit       chan struct{}                // exit signals the profiler to stop; it is closed after stopping
	stopOnce   sync.Once                    // stopOnce ensures the profiler is stopped exactly once.
	wg         sync.WaitGroup               // wg waits for all goroutines to exit when stopping.
	met        *metrics                     // metric collector state
	prev       map[string]*pprofile.Profile // previous collection results for delta profiling
	telemetry  *telemetry.Client
	seq        uint64 // seq is the value of the profile_seq tag

	testHooks testHooks
}

// testHooks are functions that are replaced during testing which would normally
// depend on accessing runtime state that is not needed/available for the test
type testHooks struct {
	startCPUProfile func(w io.Writer) error
	stopCPUProfile  func()
	lookupProfile   func(name string, w io.Writer, debug int) error
}

func (p *profiler) startCPUProfile(w io.Writer) error {
	if p.testHooks.startCPUProfile != nil {
		return p.testHooks.startCPUProfile(w)
	}
	return pprof.StartCPUProfile(w)
}

func (p *profiler) stopCPUProfile() {
	if p.testHooks.startCPUProfile != nil {
		p.testHooks.stopCPUProfile()
		return
	}
	pprof.StopCPUProfile()
}

func (p *profiler) lookupProfile(name string, w io.Writer, debug int) error {
	if p.testHooks.lookupProfile != nil {
		return p.testHooks.lookupProfile(name, w, debug)
	}
	prof := pprof.Lookup(name)
	if prof == nil {
		return errors.New("profile not found")
	}
	return prof.WriteTo(w, debug)
}

// newProfiler creates a new, unstarted profiler.
func newProfiler(opts ...Option) (*profiler, error) {
	cfg, err := defaultConfig()
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(cfg)
	}
	// TODO(fg) remove this after making expGoroutineWaitProfile public.
	if os.Getenv("DD_PROFILING_WAIT_PROFILE") != "" {
		cfg.addProfileType(expGoroutineWaitProfile)
	}
	// Agentless upload is disabled by default as of v1.30.0, but
	// WithAgentlessUpload can be used to enable it for testing and debugging.
	if cfg.agentless {
		if !isAPIKeyValid(cfg.apiKey) {
			return nil, errors.New("profiler.WithAgentlessUpload requires a valid API key. Use profiler.WithAPIKey or the DD_API_KEY env variable to set it")
		}
		// Always warn people against using this mode for now. All customers should
		// use agent based uploading at this point.
		log.Warn("profiler.WithAgentlessUpload is currently for internal usage only and not officially supported.")
		cfg.targetURL = cfg.apiURL
	} else {
		// Historically people could use an API Key to enable agentless uploading.
		// As of v1.30.0 customers the default behavior is to use agent based
		// uploading regardless of the presence of an API key. So if we see an API
		// key configured, we warn the customers that this is probably a
		// misconfiguration.
		if cfg.apiKey != "" {
			log.Warn("You are currently setting profiler.WithAPIKey or the DD_API_KEY env variable, but as of dd-trace-go v1.30.0 this value is getting ignored by the profiler. Please see the profiler.WithAPIKey go docs and verify that your integration is still working. If you can't remove DD_API_KEY from your environment, you can use WithAPIKey(\"\") to silence this warning.")
		}
		cfg.targetURL = cfg.agentURL
	}
	if cfg.hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			if cfg.targetURL == cfg.apiURL {
				return nil, fmt.Errorf("could not obtain hostname: %v", err)
			}
			log.Warn("unable to look up hostname: %v", err)
		}
		cfg.hostname = hostname
	}
	// uploadTimeout defaults to DefaultUploadTimeout, but in theory a user might
	// set it to 0 or a negative value. However, it's not clear what this should
	// mean, and most meanings we could assign seem to be bad: Not having a
	// timeout is dangerous, having a timeout that fires immediately breaks
	// uploading, and silently defaulting to the default timeout is confusing.
	// So let's just stay clear of all of this by not allowing such values.
	//
	// see similar discussion: https://github.com/golang/go/issues/39177
	if cfg.uploadTimeout <= 0 {
		return nil, fmt.Errorf("invalid upload timeout, must be > 0: %s", cfg.uploadTimeout)
	}
	for pt := range cfg.types {
		if _, ok := profileTypes[pt]; !ok {
			return nil, fmt.Errorf("unknown profile type: %d", pt)
		}
	}
	if cfg.logStartup {
		logStartup(cfg)
	}

	p := profiler{
		cfg:  cfg,
		out:  make(chan batch, outChannelSize),
		exit: make(chan struct{}),
		met:  newMetrics(),
		prev: make(map[string]*pprofile.Profile),
	}
	p.uploadFunc = p.upload
	p.telemetry = &telemetry.Client{
		APIKey:    cfg.apiKey,
		Namespace: telemetry.NamespaceProfilers,
		Service:   cfg.service,
		Env:       cfg.env,
		Client:    p.cfg.httpClient, // use the profiler's http.Client, gives us UDS if that's used
	}

	// For the initial release, prefer off-by-default rather than
	// on-by-default
	if !internal.BoolEnv("DD_INSTRUMENTATION_TELEMETRY_ENABLED", false) {
		p.telemetry.Disabled = true
	}

	// For the URL, uploading through agent goes through
	//	${AGENT_URL}/telemetry/proxy/api/v2/apmtelemetry
	// for agentless (which we technically don't support):
	//	https://instrumentation-telemetry-intake.datadoghq.com/api/v2/apmtelemetry
	// with an API key
	//
	// TODO: move this logic into the telemetry package, make the URL field
	// unnecessary?
	if cfg.agentless {
		p.telemetry.URL = "https://instrumentation-telemetry-intake.datadoghq.com/api/v2/apmtelemetry"
	} else {
		// TODO: check agent /info endpoint to see if the agent is
		// sufficiently recent to support this endpiont? overkill?
		u, err := url.Parse(cfg.agentURL)
		if err == nil {
			u.Path = "/telemetry/proxy/api/v2/apmtelemetry"
			p.telemetry.URL = u.String()
		} else {
			log.Warn("Agent URL %s is invalid, not starting telemetry", cfg.agentURL)
			p.telemetry.Disabled = true
		}
	}
	return &p, nil
}

// run runs the profiler.
func (p *profiler) run() {
	profileEnabled := func(t ProfileType) bool {
		_, ok := p.cfg.types[t]
		return ok
	}
	p.telemetry.Start(
		// Integrations are part of this API to match the telemetry API,
		// but there aren't really "integrations" for the profiler.
		// Note that if we did have integrations, we'd need a mechanism
		// to report the individual packages. runtime/debug.BuildInfo
		// only shows *modules* that we depend on, not individual
		// packages.
		[]telemetry.Integration{},
		[]telemetry.Configuration{
			{Name: "delta_profiles", Value: p.cfg.deltaProfiles},
			{Name: "agentless", Value: p.cfg.agentless},
			{Name: "profile_period", Value: p.cfg.period.String()},
			{Name: "cpu_duration", Value: p.cfg.cpuDuration.String()},
			{Name: "cpu_profile_rate", Value: p.cfg.cpuProfileRate},
			{Name: "block_profile_rate", Value: p.cfg.blockRate},
			{Name: "mutex_profile_fraction", Value: p.cfg.mutexFraction},
			{Name: "max_goroutines_wait", Value: p.cfg.maxGoroutinesWait},
			{Name: "cpu_profile_enabled", Value: profileEnabled(CPUProfile)},
			{Name: "heap_profile_enabled", Value: profileEnabled(HeapProfile)},
			{Name: "block_profile_enabled", Value: profileEnabled(BlockProfile)},
			{Name: "mutex_profile_enabled", Value: profileEnabled(MutexProfile)},
			{Name: "goroutine_profile_enabled", Value: profileEnabled(GoroutineProfile)},
			{Name: "goroutine_wait_profile_enabled", Value: profileEnabled(expGoroutineWaitProfile)},
			{Name: "upload_timeout", Value: p.cfg.uploadTimeout.String()},
		},
	)

	if profileEnabled(MutexProfile) {
		runtime.SetMutexProfileFraction(p.cfg.mutexFraction)
	}
	if profileEnabled(BlockProfile) {
		runtime.SetBlockProfileRate(p.cfg.blockRate)
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		tick := time.NewTicker(p.cfg.period)
		defer tick.Stop()
		p.met.reset(now()) // collect baseline metrics at profiler start
		p.collect(tick.C)
	}()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.send()
	}()
}

// collect runs the profile types found in the configuration whenever the ticker receives
// an item.
func (p *profiler) collect(ticker <-chan time.Time) {
	defer close(p.out)
	var (
		// mu guards completed
		mu        sync.Mutex
		completed []*profile
		wg        sync.WaitGroup
	)
	for {
		now := now()
		bat := batch{
			seq:   p.seq,
			host:  p.cfg.hostname,
			start: now,
			// NB: while this is technically wrong in that it does not
			// record the actual start and end timestamps for the batch,
			// it is how the backend understands the client-side
			// configured CPU profile duration: (start-end).
			end: now.Add(p.cfg.cpuDuration),
		}
		p.seq++

		completed = completed[:0]
		for _, t := range p.enabledProfileTypes() {
			wg.Add(1)
			go func(t ProfileType) {
				defer wg.Done()
				profs, err := p.runProfile(t)
				if err != nil {
					log.Error("Error getting %s profile: %v; skipping.", t, err)
					tags := append(p.cfg.tags.Slice(), t.Tag())
					p.cfg.statsd.Count("datadog.profiling.go.collect_error", 1, tags, 1)
				}
				mu.Lock()
				defer mu.Unlock()
				completed = append(completed, profs...)
			}(t)
		}
		wg.Wait()
		for _, prof := range completed {
			bat.addProfile(prof)
		}
		p.enqueueUpload(bat)
		select {
		case <-ticker:
		case <-p.exit:
			return
		}
	}
}

// enabledProfileTypes returns the enabled profile types in a deterministic
// order. The CPU profile always comes first because people might spot
// interesting events in there and then try to look for the counter-part event
// in the mutex/heap/block profile. Deterministic ordering is also important
// for delta profiles, otherwise they'd cover varying profiling periods.
func (p *profiler) enabledProfileTypes() []ProfileType {
	order := []ProfileType{
		CPUProfile,
		HeapProfile,
		BlockProfile,
		MutexProfile,
		GoroutineProfile,
		expGoroutineWaitProfile,
		MetricsProfile,
	}
	enabled := []ProfileType{}
	for _, t := range order {
		if _, ok := p.cfg.types[t]; ok {
			enabled = append(enabled, t)
		}
	}
	return enabled
}

// enqueueUpload pushes a batch of profiles onto the queue to be uploaded. If there is no room, it will
// evict the oldest profile to make some. Typically a batch would be one of each enabled profile.
func (p *profiler) enqueueUpload(bat batch) {
	for {
		select {
		case p.out <- bat:
			return // 👍
		default:
			// queue is full; evict oldest
			select {
			case <-p.out:
				p.cfg.statsd.Count("datadog.profiling.go.queue_full", 1, p.cfg.tags.Slice(), 1)
				log.Warn("Evicting one profile batch from the upload queue to make room.")
			default:
				// this case should be almost impossible to trigger, it would require a
				// full p.out to completely drain within nanoseconds or extreme
				// scheduling decisions by the runtime.
			}
		}
	}
}

// send takes profiles from the output queue and uploads them.
func (p *profiler) send() {
	for {
		select {
		case <-p.exit:
			return
		case bat := <-p.out:
			if err := p.outputDir(bat); err != nil {
				log.Error("Failed to output profile to dir: %v", err)
			}
			if err := p.uploadFunc(bat); err != nil {
				log.Error("Failed to upload profile: %v", err)
			}
		}
	}
}

func (p *profiler) outputDir(bat batch) error {
	if p.cfg.outputDir == "" {
		return nil
	}
	// Basic ISO 8601 Format in UTC as the name for the directories.
	dir := bat.end.UTC().Format("20060102T150405Z")
	dirPath := filepath.Join(p.cfg.outputDir, dir)
	// 0755 is what mkdir does, should be reasonable for the use cases here.
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}

	for _, prof := range bat.profiles {
		filePath := filepath.Join(dirPath, prof.name)
		// 0644 is what touch does, should be reasonable for the use cases here.
		if err := os.WriteFile(filePath, prof.data, 0644); err != nil {
			return err
		}
	}
	return nil
}

// interruptibleSleep sleeps for the given duration or until interrupted by the
// p.exit channel being closed.
func (p *profiler) interruptibleSleep(d time.Duration) {
	select {
	case <-p.exit:
	case <-time.After(d):
	}
}

// stop stops the profiler.
func (p *profiler) stop() {
	p.stopOnce.Do(func() {
		close(p.exit)
		p.telemetry.Stop()
	})
	p.wg.Wait()
}

// StatsdClient implementations can count and time certain event occurrences that happen
// in the profiler.
type StatsdClient interface {
	// Count counts how many times an event happened, at the given rate using the given tags.
	Count(event string, times int64, tags []string, rate float64) error
	// Timing creates a distribution of the values registered as the duration of a certain event.
	Timing(event string, duration time.Duration, tags []string, rate float64) error
}
