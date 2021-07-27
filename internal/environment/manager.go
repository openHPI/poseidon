package environment

import (
	_ "embed"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/jobspec2"
	"github.com/hashicorp/nomad/nomad/structs"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/logging"
	"os"
	"strconv"
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

	// CreateOrUpdate creates/updates an execution environment on the executor.
	// If the job was created, the returned boolean is true, if it was updated, it is false.
	// If err is not nil, that means the environment was neither created nor updated.
	CreateOrUpdate(
		id runner.EnvironmentID,
		request dto.ExecutionEnvironmentRequest,
	) (bool, error)
}

func NewNomadEnvironmentManager(
	runnerManager runner.Manager,
	apiClient nomad.ExecutorAPI,
	templateJobFile string,
) (*NomadEnvironmentManager, error) {
	if err := loadTemplateEnvironmentJobHCL(templateJobFile); err != nil {
		return nil, err
	}
	templateEnvironmentJob, err := parseJob(templateEnvironmentJobHCL)
	if err != nil {
		return nil, err
	}
	m := &NomadEnvironmentManager{runnerManager, apiClient, *templateEnvironmentJob}
	if err := m.Load(); err != nil {
		log.WithError(err).Error("Error recovering the execution environments")
	}
	runnerManager.Load()
	return m, nil
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

type NomadEnvironmentManager struct {
	runnerManager          runner.Manager
	api                    nomad.ExecutorAPI
	templateEnvironmentJob nomadApi.Job
}

func (m *NomadEnvironmentManager) CreateOrUpdate(
	id runner.EnvironmentID,
	request dto.ExecutionEnvironmentRequest,
) (bool, error) {
	templateJob, err := m.api.RegisterTemplateJob(&m.templateEnvironmentJob, runner.TemplateJobID(id),
		request.PrewarmingPoolSize, request.CPULimit, request.MemoryLimit,
		request.Image, request.NetworkAccess, request.ExposedPorts)

	if err != nil {
		return false, fmt.Errorf("error registering template job in API: %w", err)
	}

	created, err := m.runnerManager.CreateOrUpdateEnvironment(id, request.PrewarmingPoolSize, templateJob, true)
	if err != nil {
		return created, fmt.Errorf("error updating environment in runner manager: %w", err)
	}
	return created, nil
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
		configTaskGroup := nomad.FindConfigTaskGroup(job)
		if configTaskGroup == nil {
			jobLogger.Info("Couldn't find config task group in job, skipping ...")
			continue
		}
		desiredIdleRunnersCount, err := strconv.Atoi(configTaskGroup.Meta[nomad.ConfigMetaPoolSizeKey])
		if err != nil {
			jobLogger.Infof("Couldn't convert pool size to int: %v, skipping ...", err)
			continue
		}
		environmentIDString, err := runner.EnvironmentIDFromTemplateJobID(*job.ID)
		if err != nil {
			jobLogger.WithError(err).Error("Couldn't retrieve environment id from template job")
		}
		environmentID, err := runner.NewEnvironmentID(environmentIDString)
		if err != nil {
			jobLogger.WithField("environmentID", environmentIDString).
				WithError(err).
				Error("Couldn't retrieve environmentID from string")
			continue
		}
		_, err = m.runnerManager.CreateOrUpdateEnvironment(environmentID, uint(desiredIdleRunnersCount), job, false)
		if err != nil {
			jobLogger.WithError(err).Info("Could not recover job.")
			continue
		}
		jobLogger.Info("Successfully recovered environment")
	}
	return nil
}

func parseJob(jobHCL string) (*nomadApi.Job, error) {
	config := jobspec2.ParseConfig{
		Body:    []byte(jobHCL),
		AllowFS: false,
		Strict:  true,
	}
	job, err := jobspec2.ParseWithConfig(&config)
	if err != nil {
		return nil, fmt.Errorf("error parsing Nomad job: %w", err)
	}

	return job, nil
}
