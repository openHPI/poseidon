package environment

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/pkg/util"
	"github.com/sirupsen/logrus"
)

// templateEnvironmentJobHCL holds our default job in HCL format.
// The default job is used when creating new job and provides
// common settings that all the jobs share.
//
//go:embed template-environment-job.hcl
var templateEnvironmentJobHCL string

var log = logging.GetLogger("environment")

type NomadEnvironmentManager struct {
	*AbstractManager
	api                    nomad.ExecutorAPI
	templateEnvironmentHCL string
}

func NewNomadEnvironmentManager(
	runnerManager runner.Manager,
	apiClient nomad.ExecutorAPI,
	templateJobFile string,
) (*NomadEnvironmentManager, error) {
	if err := loadTemplateEnvironmentJobHCL(templateJobFile); err != nil {
		return nil, err
	}

	m := &NomadEnvironmentManager{
		&AbstractManager{nil, runnerManager},
		apiClient, templateEnvironmentJobHCL,
	}
	return m, nil
}

func (m *NomadEnvironmentManager) Get(ctx context.Context, environmentID dto.EnvironmentID, fetch bool) (
	executionEnvironment runner.ExecutionEnvironment, err error,
) {
	executionEnvironment, ok := m.runnerManager.GetEnvironment(environmentID)

	if fetch {
		fetchedEnvironment, err := fetchEnvironment(ctx, environmentID, m.api)

		switch {
		case err != nil:
			return nil, err
		case fetchedEnvironment == nil:
			_, err = m.Delete(environmentID)
			if err != nil {
				return nil, err
			}
			ok = false
		case !ok:
			m.runnerManager.StoreEnvironment(fetchedEnvironment)
			executionEnvironment = fetchedEnvironment
			ok = true
		default:
			executionEnvironment.SetConfigFrom(fetchedEnvironment)
			// We destroy only this (second) local reference to the environment.
			err = fetchedEnvironment.Delete(runner.ErrDestroyedAndReplaced)
			if err != nil {
				log.WithError(err).Warn("Failed to remove environment locally")
			}
		}
	}

	if !ok {
		err = runner.ErrUnknownExecutionEnvironment
	}
	return executionEnvironment, err
}

func (m *NomadEnvironmentManager) List(ctx context.Context, fetch bool) ([]runner.ExecutionEnvironment, error) {
	if fetch {
		if err := m.fetchEnvironments(ctx); err != nil {
			return nil, err
		}
	}
	return m.runnerManager.ListEnvironments(), nil
}

func (m *NomadEnvironmentManager) CreateOrUpdate(
	ctx context.Context, id dto.EnvironmentID, request dto.ExecutionEnvironmentRequest,
) (created bool, err error) {
	// Check if execution environment is already existing (in the local memory).
	environment, isExistingEnvironment := m.runnerManager.GetEnvironment(id)
	if isExistingEnvironment {
		// Remove existing environment to force downloading the newest Docker image.
		// See https://github.com/openHPI/poseidon/issues/69
		err = environment.Delete(runner.ErrEnvironmentUpdated)
		if err != nil {
			return false, fmt.Errorf("failed to remove the environment: %w", err)
		}
	}

	// Create a new environment with the given request options.
	environment, err = NewNomadEnvironmentFromRequest(ctx, m.api, m.templateEnvironmentHCL, id, request)
	if err != nil {
		return false, fmt.Errorf("error creating Nomad environment: %w", err)
	}

	// Keep a copy of environment specification in memory.
	m.runnerManager.StoreEnvironment(environment)

	// Register template Job with Nomad.
	logging.StartSpan(ctx, "env.update.register", "Register Environment", func(_ context.Context, _ *sentry.Span) {
		err = environment.Register()
	})
	if err != nil {
		return false, fmt.Errorf("error registering template job in API: %w", err)
	}

	// Launch idle runners based on the template job.
	logging.StartSpan(ctx, "env.update.poolsize", "Apply Prewarming Pool Size", func(_ context.Context, _ *sentry.Span) {
		err = environment.ApplyPrewarmingPoolSize()
	})
	if err != nil {
		return false, fmt.Errorf("error scaling template job in API: %w", err)
	}

	return !isExistingEnvironment, nil
}

// KeepEnvironmentsSynced loads all environments, runner existing at Nomad, and watches Nomad events to handle further changes.
func (m *NomadEnvironmentManager) KeepEnvironmentsSynced(ctx context.Context, synchronizeRunners func(ctx context.Context) error) {
	err := util.RetryConstantAttemptsWithContext(ctx, math.MaxInt, func() error {
		// Load Environments
		if err := m.load(ctx); err != nil {
			log.WithContext(ctx).WithError(err).
				WithField(logging.SentryFingerprintFieldKey, []string{"{{ default }}", "environments"}).
				Warn("Loading Environments failed! Retrying...")
			return err
		}

		// Load Runners and keep them synchronized.
		if err := synchronizeRunners(ctx); err != nil && ctx.Err() == nil {
			log.WithContext(ctx).WithError(err).
				WithField(logging.SentryFingerprintFieldKey, []string{"{{ default }}", "runners"}).
				Warn("Loading and synchronizing Runners failed! Retrying...")
			return err
		}

		return nil
	})
	level := logrus.InfoLevel
	if err != nil && ctx.Err() == nil {
		level = logrus.FatalLevel
	}
	log.WithContext(ctx).WithError(err).Log(level, "Stopped KeepEnvironmentsSynced")
}

func (m *NomadEnvironmentManager) fetchEnvironments(ctx context.Context) error {
	remoteEnvironmentList, err := m.api.LoadEnvironmentJobs()
	if err != nil {
		return fmt.Errorf("failed fetching environments: %w", err)
	}
	remoteEnvironments := make(map[string]*nomadApi.Job)

	// Update local environments from remote environments.
	for _, job := range remoteEnvironmentList {
		remoteEnvironments[*job.ID] = job
		id, err := nomad.EnvironmentIDFromTemplateJobID(*job.ID)
		if err != nil {
			return fmt.Errorf("cannot parse environment id: %w", err)
		}

		if localEnvironment, ok := m.runnerManager.GetEnvironment(id); ok {
			fetchedEnvironment := newNomadEnvironmentFromJob(ctx, job, m.api)
			localEnvironment.SetConfigFrom(fetchedEnvironment)
			// We destroy only this (second) local reference to the environment.
			if err = fetchedEnvironment.Delete(runner.ErrDestroyedAndReplaced); err != nil {
				log.WithError(err).Warn("Failed to remove environment locally")
			}
		} else {
			m.runnerManager.StoreEnvironment(newNomadEnvironmentFromJob(ctx, job, m.api))
		}
	}

	// Remove local environments that are not remote environments.
	for _, localEnvironment := range m.runnerManager.ListEnvironments() {
		if _, ok := remoteEnvironments[localEnvironment.ID().ToString()]; !ok {
			err := localEnvironment.Delete(runner.ErrLocalDestruction)
			log.WithError(err).Warn("Failed to remove environment locally")
		}
	}
	return nil
}

// Load recovers all environments from the Jobs in Nomad.
// As it replaces the environments the idle runners stored are not tracked anymore.
func (m *NomadEnvironmentManager) load(ctx context.Context) error {
	log.Info("Loading environments")
	// We have to destroy the environments first as otherwise they would just be replaced and old goroutines might stay running.
	for _, environment := range m.runnerManager.ListEnvironments() {
		err := environment.Delete(runner.ErrDestroyedAndReplaced)
		if err != nil {
			log.WithError(err).Warn("Failed deleting environment locally. Possible memory leak")
		}
	}

	templateJobs, err := m.api.LoadEnvironmentJobs()
	if err != nil {
		return fmt.Errorf("couldn't load template jobs: %w", err)
	}

	for _, job := range templateJobs {
		jobLogger := log.WithField("jobID", *job.ID)
		if *job.Status != structs.JobStatusRunning {
			jobLogger.Info("Job not running, skipping...")
			continue
		}
		configTaskGroup := nomad.FindAndValidateConfigTaskGroup(job)
		if configTaskGroup == nil {
			jobLogger.Error("FindAndValidateConfigTaskGroup is not creating the task group")
			continue
		}
		environment := newNomadEnvironmentFromJob(ctx, job, m.api)
		m.runnerManager.StoreEnvironment(environment)
		jobLogger.Info("Successfully recovered environment")
	}
	return nil
}

// newNomadEnvironmentFromJob creates a Nomad environment from the passed Nomad job definition.
// Be aware that the passed context does not cancel the environment. The environment needs to be `Delete`d.
func newNomadEnvironmentFromJob(ctx context.Context, job *nomadApi.Job, apiClient nomad.ExecutorAPI) *NomadEnvironment {
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	e := &NomadEnvironment{
		apiClient: apiClient,
		jobHCL:    templateEnvironmentJobHCL,
		job:       job,
		ctx:       ctx,
		cancel:    cancel,
	}
	e.idleRunners = storage.NewMonitoredLocalStorage[runner.Runner](
		ctx, monitoring.MeasurementIdleRunnerNomad, runner.MonitorEnvironmentID[runner.Runner](e.ID()), time.Minute)
	return e
}

// loadTemplateEnvironmentJobHCL loads the template environment job HCL from the given path.
// If the path is empty, the embedded default file is used.
func loadTemplateEnvironmentJobHCL(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error loading template environment job: %w", err)
	}
	templateEnvironmentJobHCL = string(data)
	return nil
}

func fetchEnvironment(ctx context.Context, requestedEnvironmentID dto.EnvironmentID, apiClient nomad.ExecutorAPI,
) (runner.ExecutionEnvironment, error) {
	environments, err := apiClient.LoadEnvironmentJobs()
	if err != nil {
		return nil, fmt.Errorf("error fetching the environment jobs: %w", err)
	}
	var fetchedEnvironment runner.ExecutionEnvironment
	for _, job := range environments {
		fetchedEnvironmentID, err := nomad.EnvironmentIDFromTemplateJobID(*job.ID)
		if err != nil {
			log.WithError(err).Warn("Cannot parse environment id of loaded environment")
			continue
		}
		if requestedEnvironmentID == fetchedEnvironmentID {
			fetchedEnvironment = newNomadEnvironmentFromJob(ctx, job, apiClient)
		}
	}
	return fetchedEnvironment, nil
}
