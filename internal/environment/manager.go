package environment

import (
	_ "embed"
	"fmt"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"os"
)

// templateEnvironmentJobHCL holds our default job in HCL format.
// The default job is used when creating new job and provides
// common settings that all the jobs share.
//go:embed template-environment-job.hcl
var templateEnvironmentJobHCL string

var log = logging.GetLogger("environment")

// Manager encapsulates API calls to the executor API for creation and deletion of execution environments.
type Manager interface {
	// Load fetches all already created execution environments from the executor and registers them at the runner manager.
	// It should be called during the startup process (e.g. on creation of the Manager).
	Load() error

	// List returns all environments known by Poseidon.
	// When `fetch` is set the environments are fetched from the executor before returning.
	List(fetch bool) ([]runner.ExecutionEnvironment, error)

	// Get returns the details of the requested environment.
	// When `fetch` is set the requested environment is fetched from the executor before returning.
	Get(id dto.EnvironmentID, fetch bool) (runner.ExecutionEnvironment, error)

	// CreateOrUpdate creates/updates an execution environment on the executor.
	// If the job was created, the returned boolean is true, if it was updated, it is false.
	// If err is not nil, that means the environment was neither created nor updated.
	CreateOrUpdate(
		id dto.EnvironmentID,
		request dto.ExecutionEnvironmentRequest,
	) (bool, error)

	// Delete removes the specified execution environment.
	// Iff the specified environment could not be found Delete returns false.
	Delete(id dto.EnvironmentID) (bool, error)
}

type NomadEnvironmentManager struct {
	runnerManager          runner.Manager
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

	m := &NomadEnvironmentManager{runnerManager, apiClient, templateEnvironmentJobHCL}
	if err := m.Load(); err != nil {
		log.WithError(err).Error("Error recovering the execution environments")
	}
	runnerManager.Load()
	return m, nil
}

func (m *NomadEnvironmentManager) Get(id dto.EnvironmentID, fetch bool) (runner.ExecutionEnvironment, error) {
	executionEnvironment, ok := m.runnerManager.GetEnvironment(id)

	if fetch {
		var fetchedEnvironment runner.ExecutionEnvironment
		environments, err := m.api.LoadEnvironmentJobs()
		if err != nil {
			return nil, fmt.Errorf("error fetching the environment jobs: %w", err)
		}
		for _, job := range environments {
			environmentID, err := nomad.EnvironmentIDFromTemplateJobID(*job.ID)
			if err != nil {
				log.WithError(err).Warn("Cannot parse environment id of loaded environment")
				continue
			}
			if id == environmentID {
				fetchedEnvironment = &NomadEnvironment{
					jobHCL:      templateEnvironmentJobHCL,
					job:         job,
					idleRunners: runner.NewLocalRunnerStorage(),
				}
			}
		}
		if fetchedEnvironment == nil {
			_, err = m.Delete(id)
			if err != nil {
				return nil, err
			}
			ok = false
		} else if !ok {
			m.runnerManager.SetEnvironment(fetchedEnvironment)
			executionEnvironment = fetchedEnvironment
			ok = true
		} else {
			executionEnvironment.SetPrewarmingPoolSize(fetchedEnvironment.PrewarmingPoolSize())
			executionEnvironment.SetCPULimit(fetchedEnvironment.CPULimit())
			executionEnvironment.SetMemoryLimit(fetchedEnvironment.MemoryLimit())
			executionEnvironment.SetImage(fetchedEnvironment.Image())
			executionEnvironment.SetNetworkAccess(fetchedEnvironment.NetworkAccess())
		}
	}

	if ok {
		return executionEnvironment, nil
	} else {
		return nil, runner.ErrUnknownExecutionEnvironment
	}
}

func (m *NomadEnvironmentManager) List(fetch bool) ([]runner.ExecutionEnvironment, error) {
	if fetch {
		err := m.Load()
		if err != nil {
			return nil, err
		}
	}
	return m.runnerManager.ListEnvironments(), nil
}

func (m *NomadEnvironmentManager) CreateOrUpdate(id dto.EnvironmentID, request dto.ExecutionEnvironmentRequest) (
	created bool, err error) {
	environment, ok := m.runnerManager.GetEnvironment(id)
	if !ok {
		environment, err = NewNomadEnvironment(m.templateEnvironmentHCL)
		if err != nil {
			return false, fmt.Errorf("error creating Nomad environment: %w", err)
		}
		environment.SetID(id)
	}

	environment.SetPrewarmingPoolSize(request.PrewarmingPoolSize)
	environment.SetCPULimit(request.CPULimit)
	environment.SetMemoryLimit(request.MemoryLimit)
	environment.SetImage(request.Image)
	created = m.runnerManager.SetEnvironment(environment)

	err = environment.Register(m.api)
	if err != nil {
		return false, fmt.Errorf("error registering template job in API: %w", err)
	}
	err = environment.UpdateRunnerSpecs(m.api)
	if err != nil {
		return false, fmt.Errorf("error updating runner jobs in API: %w", err)
	}
	err = environment.Scale(m.api)
	if err != nil {
		return false, fmt.Errorf("error scaling template job in API: %w", err)
	}

	return created, nil
}

func (m *NomadEnvironmentManager) Delete(id dto.EnvironmentID) (bool, error) {
	executionEnvironment, ok := m.runnerManager.GetEnvironment(id)
	if !ok {
		return false, nil
	}
	m.runnerManager.DeleteEnvironment(id)
	return true, executionEnvironment.Delete(m.api)
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
		configTaskGroup := nomad.FindOrCreateConfigTaskGroup(job)
		if configTaskGroup == nil {
			jobLogger.Info("Couldn't find config task group in job, skipping ...")
			continue
		}
		environment := &NomadEnvironment{
			jobHCL:      templateEnvironmentJobHCL,
			job:         job,
			idleRunners: runner.NewLocalRunnerStorage(),
		}
		m.runnerManager.SetEnvironment(environment)
		jobLogger.Info("Successfully recovered environment")
	}
	return nil
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
