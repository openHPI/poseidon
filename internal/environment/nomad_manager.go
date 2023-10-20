package environment

import (
	"context"
	_ "embed"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/pkg/util"
	"os"
	"time"
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
	ctx context.Context,
) (*NomadEnvironmentManager, error) {
	if err := loadTemplateEnvironmentJobHCL(templateJobFile); err != nil {
		return nil, err
	}

	m := &NomadEnvironmentManager{&AbstractManager{nil, runnerManager},
		apiClient, templateEnvironmentJobHCL}
	if err := util.RetryExponentialWithContext(ctx, func() error { return m.Load() }); err != nil {
		log.WithError(err).Error("Error recovering the execution environments")
	}
	runnerManager.Load()
	return m, nil
}

func (m *NomadEnvironmentManager) Get(id dto.EnvironmentID, fetch bool) (
	executionEnvironment runner.ExecutionEnvironment, err error) {
	executionEnvironment, ok := m.runnerManager.GetEnvironment(id)

	if fetch {
		fetchedEnvironment, err := fetchEnvironment(id, m.api)
		switch {
		case err != nil:
			return nil, err
		case fetchedEnvironment == nil:
			_, err = m.Delete(id)
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
			err = fetchedEnvironment.Delete(true)
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

func (m *NomadEnvironmentManager) List(fetch bool) ([]runner.ExecutionEnvironment, error) {
	if fetch {
		if err := m.fetchEnvironments(); err != nil {
			return nil, err
		}
	}
	return m.runnerManager.ListEnvironments(), nil
}

func (m *NomadEnvironmentManager) fetchEnvironments() error {
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
			fetchedEnvironment := newNomadEnvironmentFromJob(job, m.api)
			localEnvironment.SetConfigFrom(fetchedEnvironment)
			// We destroy only this (second) local reference to the environment.
			if err = fetchedEnvironment.Delete(true); err != nil {
				log.WithError(err).Warn("Failed to remove environment locally")
			}
		} else {
			m.runnerManager.StoreEnvironment(newNomadEnvironmentFromJob(job, m.api))
		}
	}

	// Remove local environments that are not remote environments.
	for _, localEnvironment := range m.runnerManager.ListEnvironments() {
		if _, ok := remoteEnvironments[localEnvironment.ID().ToString()]; !ok {
			err := localEnvironment.Delete(true)
			log.WithError(err).Warn("Failed to remove environment locally")
		}
	}
	return nil
}

func (m *NomadEnvironmentManager) CreateOrUpdate(
	id dto.EnvironmentID, request dto.ExecutionEnvironmentRequest, ctx context.Context) (created bool, err error) {
	// Check if execution environment is already existing (in the local memory).
	environment, isExistingEnvironment := m.runnerManager.GetEnvironment(id)
	if isExistingEnvironment {
		// Remove existing environment to force downloading the newest Docker image.
		// See https://github.com/openHPI/poseidon/issues/69
		err = environment.Delete(false)
		if err != nil {
			return false, fmt.Errorf("failed to remove the environment: %w", err)
		}
	}

	// Create a new environment with the given request options.
	environment, err = NewNomadEnvironmentFromRequest(m.api, m.templateEnvironmentHCL, id, request)
	if err != nil {
		return false, fmt.Errorf("error creating Nomad environment: %w", err)
	}

	// Keep a copy of environment specification in memory.
	m.runnerManager.StoreEnvironment(environment)

	// Register template Job with Nomad.
	logging.StartSpan("env.update.register", "Register Environment", ctx, func(_ context.Context) {
		err = environment.Register()
	})
	if err != nil {
		return false, fmt.Errorf("error registering template job in API: %w", err)
	}

	// Launch idle runners based on the template job.
	logging.StartSpan("env.update.poolsize", "Apply Prewarming Pool Size", ctx, func(_ context.Context) {
		err = environment.ApplyPrewarmingPoolSize()
	})
	if err != nil {
		return false, fmt.Errorf("error scaling template job in API: %w", err)
	}

	return !isExistingEnvironment, nil
}

func (m *NomadEnvironmentManager) Load() error {
	templateJobs, err := m.api.LoadEnvironmentJobs()
	if err != nil {
		return fmt.Errorf("couldn't load template jobs: %w", err)
	}

	for _, job := range templateJobs {
		jobLogger := log.WithField("jobID", *job.ID)
		if *job.Status != structs.JobStatusRunning {
			jobLogger.Info("Job not running, skipping ...")
			continue
		}
		configTaskGroup := nomad.FindAndValidateConfigTaskGroup(job)
		if configTaskGroup == nil {
			jobLogger.Error("FindAndValidateConfigTaskGroup is not creating the task group")
			continue
		}
		environment := newNomadEnvironmentFromJob(job, m.api)
		m.runnerManager.StoreEnvironment(environment)
		jobLogger.Info("Successfully recovered environment")
	}
	return nil
}

// newNomadEnvironmentFromJob creates a Nomad environment from the passed Nomad job definition.
func newNomadEnvironmentFromJob(job *nomadApi.Job, apiClient nomad.ExecutorAPI) *NomadEnvironment {
	ctx, cancel := context.WithCancel(context.Background())
	e := &NomadEnvironment{
		apiClient: apiClient,
		jobHCL:    templateEnvironmentJobHCL,
		job:       job,
		ctx:       ctx,
		cancel:    cancel,
	}
	e.idleRunners = storage.NewMonitoredLocalStorage[runner.Runner](monitoring.MeasurementIdleRunnerNomad,
		runner.MonitorEnvironmentID[runner.Runner](e.ID()), time.Minute, ctx)
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

func fetchEnvironment(id dto.EnvironmentID, apiClient nomad.ExecutorAPI) (runner.ExecutionEnvironment, error) {
	environments, err := apiClient.LoadEnvironmentJobs()
	if err != nil {
		return nil, fmt.Errorf("error fetching the environment jobs: %w", err)
	}
	var fetchedEnvironment runner.ExecutionEnvironment
	for _, job := range environments {
		environmentID, err := nomad.EnvironmentIDFromTemplateJobID(*job.ID)
		if err != nil {
			log.WithError(err).Warn("Cannot parse environment id of loaded environment")
			continue
		}
		if id == environmentID {
			fetchedEnvironment = newNomadEnvironmentFromJob(job, apiClient)
		}
	}
	return fetchedEnvironment, nil
}
