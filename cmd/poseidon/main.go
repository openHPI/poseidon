package main

import (
	"context"
	"errors"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"syscall"
	"time"
)

var (
	gracefulShutdownWait = 15 * time.Second
	log                  = logging.GetLogger("main")
	// If pgoEnabled is true, the binary was built with PGO enabled.
	// This is set during compilation with our Makefile as a STRING.
	pgoEnabled = "false"
)

func getVcsRevision() string {
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

	if vcsModified {
		return vcsRevision + "-modified"
	} else {
		return vcsRevision
	}
}

func initSentry(options *sentry.ClientOptions, profilingEnabled bool) {
	if options.Release == "" {
		commit := getVcsRevision()
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
	if options.Enabled {
		profile, err := os.Create(options.File)
		if err != nil {
			log.WithError(err).Error("Error while opening the profile file")
		}

		log.Debug("Starting CPU profiler")
		if err := pprof.StartCPUProfile(profile); err != nil {
			log.WithError(err).Error("Error while starting the CPU profiler!!")
		}

		cancel = func() {
			if options.Enabled {
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

func runServer(server *http.Server, cancel context.CancelFunc) {
	defer cancel()
	defer shutdownSentry() // shutdownSentry must be executed in the main goroutine.

	log.WithField("address", server.Addr).Info("Starting server")
	var err error
	if config.Config.Server.TLS.Active {
		server.TLSConfig = config.TLSConfig
		log.
			WithField("CertFile", config.Config.Server.TLS.CertFile).
			WithField("KeyFile", config.Config.Server.TLS.KeyFile).
			Debug("Using TLS")
		err = server.ListenAndServeTLS(config.Config.Server.TLS.CertFile, config.Config.Server.TLS.KeyFile)
	} else {
		err = server.ListenAndServe()
	}
	if err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			log.WithError(err).Info("Server closed")
		} else {
			log.WithError(err).Fatal("Error during listening and serving")
		}
	}
}

type managerCreator func() (runnerManager runner.Manager, environmentManager environment.ManagerHandler)

// createManagerHandler adds the managers of the passed managerCreator to the chain of responsibility.
func createManagerHandler(handler managerCreator, enabled bool,
	nextRunnerManager runner.Manager, nextEnvironmentManager environment.ManagerHandler) (
	runnerManager runner.Manager, environmentManager environment.ManagerHandler) {
	if !enabled {
		return nextRunnerManager, nextEnvironmentManager
	}

	runnerManager, environmentManager = handler()
	runnerManager.SetNextHandler(nextRunnerManager)
	environmentManager.SetNextHandler(nextEnvironmentManager)
	return runnerManager, environmentManager
}

func createNomadManager() (runnerManager runner.Manager, environmentManager environment.ManagerHandler) {
	// API initialization
	nomadAPIClient, err := nomad.NewExecutorAPI(&config.Config.Nomad)
	if err != nil {
		log.WithError(err).WithField("nomad config", config.Config.Nomad).Fatal("Error creating Nomad API client")
	}

	runnerManager = runner.NewNomadRunnerManager(nomadAPIClient, context.Background())
	environmentManager, err = environment.
		NewNomadEnvironmentManager(runnerManager, nomadAPIClient, config.Config.Server.TemplateJobFile)
	if err != nil {
		log.WithError(err).Fatal("Error initializing environment manager")
	}
	return runnerManager, environmentManager
}

func createAWSManager() (runnerManager runner.Manager, environmentManager environment.ManagerHandler) {
	runnerManager = runner.NewAWSRunnerManager()
	return runnerManager, environment.NewAWSEnvironmentManager(runnerManager)
}

// initServer builds the http server and configures it with the chain of responsibility for multiple managers.
func initServer() *http.Server {
	runnerManager, environmentManager := createManagerHandler(createNomadManager, config.Config.Nomad.Enabled,
		nil, nil)
	runnerManager, environmentManager = createManagerHandler(createAWSManager, config.Config.AWS.Enabled,
		runnerManager, environmentManager)

	handler := api.NewRouter(runnerManager, environmentManager)
	sentryHandler := sentryhttp.New(sentryhttp.Options{}).Handle(handler)

	return &http.Server{
		Addr: config.Config.Server.URL().Host,
		// A WriteTimeout would prohibit long-running requests such as creating an execution environment.
		// See also https://github.com/openHPI/poseidon/pull/68.
		// WriteTimeout: time.Second * 15,
		ReadHeaderTimeout: time.Second * 15,
		ReadTimeout:       time.Second * 15,
		IdleTimeout:       time.Second * 60,
		Handler:           sentryHandler,
	}
}

// shutdownOnOSSignal listens for a signal from the operating system
// When receiving a signal the server shuts down but waits up to 15 seconds to close remaining connections.
func shutdownOnOSSignal(server *http.Server, ctx context.Context, stopProfiling func()) {
	// wait for SIGINT
	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, syscall.SIGINT, syscall.SIGTERM)

	// wait for SIGUSR1
	writeProfileSignal := make(chan os.Signal, 1)
	signal.Notify(writeProfileSignal, syscall.SIGUSR1)

	select {
	case <-ctx.Done():
		os.Exit(1)
	case <-writeProfileSignal:
		log.Info("Received SIGUSR1 ...")

		stopProfiling()
		// Continue listening on signals and replace `stopProfiling` with an empty function
		shutdownOnOSSignal(server, ctx, func() {})
	case <-shutdownSignals:
		log.Info("Received SIGINT, shutting down ...")

		defer stopProfiling()
		ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownWait)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.WithError(err).Warn("error shutting server down")
		}
	}
}

func main() {
	if err := config.InitConfig(); err != nil {
		log.WithError(err).Warn("Could not initialize configuration")
	}
	logging.InitializeLogging(config.Config.Logger.Level, config.Config.Logger.Formatter)
	initSentry(&config.Config.Sentry, config.Config.Profiling.Enabled)

	cancelInflux := monitoring.InitializeInfluxDB(&config.Config.InfluxDB)
	defer cancelInflux()

	stopProfiling := initProfiling(config.Config.Profiling)

	ctx, cancel := context.WithCancel(context.Background())
	server := initServer()
	go runServer(server, cancel)
	shutdownOnOSSignal(server, ctx, stopProfiling)
}
