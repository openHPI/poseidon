package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/activation"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/gorilla/mux"
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"golang.org/x/sys/unix"
)

var (
	gracefulShutdownWait = 15 * time.Second
	log                  = logging.GetLogger("main")
	// If pgoEnabled is true, the binary was built with PGO enabled.
	// This is set during compilation with our Makefile as a STRING.
	pgoEnabled = "false"
)

func getVcsRevision(short bool) string {
	vcsRevision := "unknown"
	vcsModified := false

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				vcsRevision = setting.Value
			} else if setting.Key == "vcs.modified" {
				var err error
				vcsModified, err = strconv.ParseBool(setting.Value)
				if err != nil {
					vcsModified = true // fallback to true, so we can see that something is wrong
					log.WithError(err).Error("Could not parse the vcs.modified setting")
				}
			}
		}
	}

	if short {
		vcsRevision = vcsRevision[:7]
	}

	if vcsModified {
		return vcsRevision + "-modified"
	} else {
		return vcsRevision
	}
}

func initializeUserAgent() {
	dto.UserAgentOut = strings.ReplaceAll(dto.UserAgentOut, dto.UserAgentVCSPlaceholder, getVcsRevision(true))
	dto.UserAgentFiltered = strings.ReplaceAll(dto.UserAgentFiltered, dto.UserAgentVCSPlaceholder, getVcsRevision(true))
	dto.UserAgentFiltered = strings.ReplaceAll(dto.UserAgentFiltered, dto.UserAgentFilterTokenPlaceholder, config.Config.Server.LoggingFilterToken)
}

func initSentry(options *sentry.ClientOptions, profilingEnabled bool) {
	if options.Release == "" {
		commit := getVcsRevision(false)
		options.Release = commit
	}

	options.BeforeSendTransaction = func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
		if event.Tags == nil {
			event.Tags = make(map[string]string)
		}
		event.Tags["go_pgo"] = pgoEnabled
		event.Tags["go_profiling"] = strconv.FormatBool(profilingEnabled)
		return event
	}

	if err := sentry.Init(*options); err != nil {
		log.Errorf("sentry.Init: %s", err)
	}
}

func shutdownSentry() {
	if err := recover(); err != nil {
		sentry.CurrentHub().Recover(err)
		sentry.Flush(logging.GracefulSentryShutdown)
	}
}

func initProfiling(options config.Profiling) (cancel func()) {
	if options.CPUEnabled {
		profile, err := os.Create(options.CPUFile)
		if err != nil {
			log.WithError(err).Error("Error while opening the profile file")
		}

		log.Debug("Starting CPU profiler")
		if err := pprof.StartCPUProfile(profile); err != nil {
			log.WithError(err).Error("Error while starting the CPU profiler!!")
		}

		cancel = func() {
			if options.CPUEnabled {
				log.Debug("Stopping CPU profiler")
				pprof.StopCPUProfile()
				if err := profile.Close(); err != nil {
					log.WithError(err).Error("Error while closing profile file")
				}
			}
		}
	} else {
		cancel = func() {}
	}
	return cancel
}

// watchMemoryAndAlert monitors the memory usage of Poseidon and sends an alert if it exceeds a threshold.
func watchMemoryAndAlert(options config.Profiling) {
	if options.MemoryInterval == 0 {
		return
	}

	var exceeded bool
	for {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		log.WithField("heap", stats.HeapAlloc).Trace("Current Memory Usage")

		const megabytesToBytes = 1000 * 1000
		if !exceeded && stats.HeapAlloc >= uint64(options.MemoryThreshold)*megabytesToBytes {
			exceeded = true
			log.WithField("heap", stats.HeapAlloc).Warn("Memory Threshold exceeded")

			err := pprof.Lookup("heap").WriteTo(os.Stderr, 1)
			if err != nil {
				log.WithError(err).Warn("Failed to log the heap profile")
			}

			err = pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
			if err != nil {
				log.WithError(err).Warn("Failed to log the goroutines")
			}
		} else if exceeded {
			exceeded = false
			log.WithField("heap", stats.HeapAlloc).Info("Memory Threshold no longer exceeded")
		}

		select {
		case <-time.After(time.Duration(options.MemoryInterval) * time.Millisecond):
			continue
		case <-context.Background().Done():
			return
		}
	}
}

func runServer(router *mux.Router, server *http.Server, cancel context.CancelFunc) {
	defer cancel()
	defer shutdownSentry() // shutdownSentry must be executed in the main goroutine.

	httpListeners := getHTTPListeners(server)
	notifySystemd(router)
	serveHTTPListeners(server, httpListeners)
}

func getHTTPListeners(server *http.Server) (httpListeners []net.Listener) {
	var err error
	if config.Config.Server.SystemdSocketActivation {
		httpListeners, err = activation.Listeners()
	} else {
		var httpListener net.Listener
		httpListener, err = net.Listen("tcp", server.Addr)
		httpListeners = append(httpListeners, httpListener)
	}
	if err != nil || httpListeners == nil || len(httpListeners) == 0 {
		log.WithError(err).
			WithField("listeners", httpListeners).
			WithField("systemd_socket", config.Config.Server.SystemdSocketActivation).
			Fatal("Failed listening to any socket")
		return nil
	}
	return httpListeners
}

func serveHTTPListeners(server *http.Server, httpListeners []net.Listener) {
	var wg sync.WaitGroup
	wg.Add(len(httpListeners))
	for _, l := range httpListeners {
		go func(listener net.Listener) {
			defer wg.Done()
			log.WithField("address", listener.Addr()).Info("Serving Listener")
			serveHTTPListener(server, listener)
		}(l)
	}
	wg.Wait()
}

func serveHTTPListener(server *http.Server, listener net.Listener) {
	var err error
	if config.Config.Server.TLS.Active {
		server.TLSConfig = config.TLSConfig
		log.WithField("CertFile", config.Config.Server.TLS.CertFile).
			WithField("KeyFile", config.Config.Server.TLS.KeyFile).
			Debug("Using TLS")
		err = server.ServeTLS(listener, config.Config.Server.TLS.CertFile, config.Config.Server.TLS.KeyFile)
	} else {
		err = server.Serve(listener)
	}

	if errors.Is(err, http.ErrServerClosed) {
		log.WithError(err).WithField("listener", listener.Addr()).Info("Server closed")
	} else {
		log.WithError(err).WithField("listener", listener.Addr()).Error("Error during listening and serving")
	}
}

func notifySystemd(router *mux.Router) {
	notify, err := daemon.SdNotify(false, daemon.SdNotifyReady)
	switch {
	case err == nil && !notify:
		log.Debug("Systemd Readiness Notification not supported")
	case err != nil:
		log.WithError(err).WithField("notify", notify).Warn("Failed notifying Readiness to Systemd")
	default:
		log.Trace("Notified Readiness to Systemd")
	}

	interval, err := daemon.SdWatchdogEnabled(false)
	if err != nil || interval == 0 {
		log.WithError(err).Error("Systemd Watchdog not supported")
		return
	}
	go systemdWatchdogLoop(context.Background(), router, interval)
}

func systemdWatchdogLoop(ctx context.Context, router *mux.Router, interval time.Duration) {
	healthRoute, err := router.Get(api.HealthPath).URL()
	if err != nil {
		log.WithError(err).Error("Failed to parse Health route")
		return
	}
	healthURL := config.Config.Server.URL().String() + healthRoute.String()

	// Workaround for certificate subject names
	unspecifiedAddresses := regexp.MustCompile(`0\.0\.0\.0|\[::]`)
	healthURL = unspecifiedAddresses.ReplaceAllString(healthURL, "localhost")

	client := &http.Client{}
	if config.Config.Server.TLS.Active {
		tlsConfig := &tls.Config{RootCAs: x509.NewCertPool()} // #nosec G402 The default MinTLSVersion is secure.
		caCertBytes, err := os.ReadFile(config.Config.Server.TLS.CAFile)
		if err != nil {
			log.WithError(err).Warn("Cannot read tls ca file")
		} else {
			ok := tlsConfig.RootCAs.AppendCertsFromPEM(caCertBytes)
			log.WithField("success", ok).Trace("Loaded CA certificate")
		}
		client.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	}

	// notificationIntervalFactor defines how many more notifications we send than required.
	const notificationIntervalFactor = 2
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval / notificationIntervalFactor):
			notifySystemdWatchdog(ctx, healthURL, client)
		}
	}
}

func notifySystemdWatchdog(ctx context.Context, healthURL string, client *http.Client) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, http.NoBody)
	if err != nil {
		return
	}

	req.Header.Set("User-Agent", dto.UserAgentFiltered)
	resp, err := client.Do(req)
	if err != nil {
		log.WithError(err).Debug("Failed watchdog health check")
		// We do not check for resp.StatusCode == 503 as Poseidon's error recovery will try to handle such errors
		// by itself. The Watchdog should just check that Poseidon handles http requests at all.
		return
	}
	_ = resp.Body.Close()

	notify, err := daemon.SdNotify(false, daemon.SdNotifyWatchdog)
	switch {
	case err == nil && !notify:
		log.Debug("Systemd Watchdog Notification not supported")
	case err != nil:
		log.WithError(err).WithField("notify", notify).Warn("Failed notifying Systemd Watchdog")
	default:
		log.Trace("Notified Systemd Watchdog")
	}
}

type managerCreator func(ctx context.Context) (
	runnerManager runner.Manager, environmentManager environment.ManagerHandler)

// createManagerHandler adds the managers of the passed managerCreator to the chain of responsibility.
func createManagerHandler(handler managerCreator, enabled bool,
	nextRunnerManager runner.Manager, nextEnvironmentManager environment.ManagerHandler, ctx context.Context) (
	runnerManager runner.Manager, environmentManager environment.ManagerHandler) {
	if !enabled {
		return nextRunnerManager, nextEnvironmentManager
	}

	runnerManager, environmentManager = handler(ctx)
	runnerManager.SetNextHandler(nextRunnerManager)
	environmentManager.SetNextHandler(nextEnvironmentManager)
	return runnerManager, environmentManager
}

func createNomadManager(ctx context.Context) (runner.Manager, environment.ManagerHandler) {
	// API initialization
	nomadAPIClient, err := nomad.NewExecutorAPI(&config.Config.Nomad)
	if err != nil {
		log.WithError(err).WithField("nomad_config", config.Config.Nomad).Fatal("Error creating Nomad API client")
	}

	runnerManager := runner.NewNomadRunnerManager(nomadAPIClient, ctx)
	environmentManager, err := environment.
		NewNomadEnvironmentManager(runnerManager, nomadAPIClient, config.Config.Server.TemplateJobFile)
	if err != nil {
		log.WithError(err).Fatal("Error initializing environment manager")
	}

	synchronizeNomad(ctx, environmentManager, runnerManager)
	return runnerManager, environmentManager
}

// synchronizeNomad starts the asynchronous synchronization background task and waits for the first environment and runner recovery.
func synchronizeNomad(ctx context.Context, environmentManager *environment.NomadEnvironmentManager, runnerManager *runner.NomadRunnerManager) {
	firstRecoveryDone := make(chan struct{})
	go environmentManager.KeepEnvironmentsSynced(func(ctx context.Context) error {
		go func(ctx context.Context) {
			// `Load` not only recover existing runners, but also applies the prewarming pool size which creates runners.
			// Therefore, the Nomad Event Stream has to be started before.
			runnerManager.Load(ctx)

			select {
			case firstRecoveryDone <- struct{}{}:
				log.Info("First Recovery Done")
			default:
			}
		}(ctx)

		// The Race Condition between the startup of the event stream and the recovery of missing runners is uncritical
		// because setting the start time of the stream is the first thing done in `SynchronizeRunners` while `Load`
		// first starts an HTTP request for each individual existing runner.

		if err := runnerManager.SynchronizeRunners(ctx); err != nil {
			return fmt.Errorf("synchronize runners failed: %w", err)
		}
		return nil
	}, ctx)

	select {
	case <-firstRecoveryDone:
	case <-ctx.Done():
	}
}

func createAWSManager(ctx context.Context) (
	runnerManager runner.Manager, environmentManager environment.ManagerHandler) {
	runnerManager = runner.NewAWSRunnerManager(ctx)
	return runnerManager, environment.NewAWSEnvironmentManager(runnerManager)
}

// initRouter builds a router that serves the API with the chain of responsibility for multiple managers.
func initRouter(ctx context.Context) *mux.Router {
	runnerManager, environmentManager := createManagerHandler(createNomadManager, config.Config.Nomad.Enabled,
		nil, nil, ctx)
	runnerManager, environmentManager = createManagerHandler(createAWSManager, config.Config.AWS.Enabled,
		runnerManager, environmentManager, ctx)

	return api.NewRouter(runnerManager, environmentManager)
}

// initServer creates a server that serves the routes provided by the router.
func initServer(router *mux.Router) *http.Server {
	sentryHandler := sentryhttp.New(sentryhttp.Options{}).Handle(router)
	const readTimeout = 15 * time.Second
	const idleTimeout = 60 * time.Second

	return &http.Server{
		Addr: config.Config.Server.URL().Host,
		// A WriteTimeout would prohibit long-running requests such as creating an execution environment.
		// See also https://github.com/openHPI/poseidon/pull/68.
		// WriteTimeout: time.Second * 15,
		ReadHeaderTimeout: readTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		Handler:           sentryHandler,
	}
}

// shutdownOnOSSignal listens for a signal from the operating system
// When receiving a signal the server shuts down but waits up to 15 seconds to close remaining connections.
func shutdownOnOSSignal(server *http.Server, ctx context.Context, stopProfiling func()) {
	// wait for SIGINT
	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, unix.SIGINT, unix.SIGTERM, unix.SIGABRT)

	// wait for SIGUSR1
	writeProfileSignal := make(chan os.Signal, 1)
	signal.Notify(writeProfileSignal, unix.SIGUSR1)

	select {
	case <-ctx.Done():
		os.Exit(1)
	case <-writeProfileSignal:
		log.Info("Received SIGUSR1...")

		stopProfiling()
		// Continue listening on signals and replace `stopProfiling` with an empty function
		shutdownOnOSSignal(server, ctx, func() {})
	case <-shutdownSignals:
		log.Info("Received SIGINT, shutting down...")

		defer stopProfiling()
		gracefulCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gracefulShutdownWait)
		defer cancel()
		if err := server.Shutdown(gracefulCtx); err != nil {
			log.WithError(err).Warn("error shutting server down")
		}
	}
}

func main() {
	if err := config.InitConfig(); err != nil {
		log.WithError(err).Warn("Could not initialize configuration")
	}
	initializeUserAgent()
	logging.InitializeLogging(config.Config.Logger.Level, config.Config.Logger.Formatter)
	initSentry(&config.Config.Sentry, config.Config.Profiling.CPUEnabled)

	cancelInflux := monitoring.InitializeInfluxDB(&config.Config.InfluxDB)
	defer cancelInflux()

	stopProfiling := initProfiling(config.Config.Profiling)
	go watchMemoryAndAlert(config.Config.Profiling)

	ctx, cancel := context.WithCancel(context.Background())
	router := initRouter(ctx)
	server := initServer(router)
	go runServer(router, server, cancel)
	shutdownOnOSSignal(server, ctx, stopProfiling)
}
