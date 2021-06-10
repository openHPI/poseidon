package environment

import (
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"strconv"
)

var log = logging.GetLogger("environment")

// Manager encapsulates API calls to the executor API for creation and deletion of execution environments.
type Manager interface {
	// CreateOrUpdate creates/updates an execution environment on the executor.
	// Iff the job was created, the returned boolean is true and the returned error is nil.
	CreateOrUpdate(
		id runner.EnvironmentID,
		request dto.ExecutionEnvironmentRequest,
	) (bool, error)

	// Delete removes the execution environment with the given id from the executor.
	Delete(id string)
}

func NewNomadEnvironmentManager(runnerManager runner.Manager, apiClient nomad.ExecutorAPI) (
	*NomadEnvironmentManager, error) {
	environmentManager := &NomadEnvironmentManager{runnerManager, apiClient, *parseJob(defaultJobHCL)}
	err := environmentManager.loadExistingEnvironments()
	return environmentManager, err
}

type NomadEnvironmentManager struct {
	runnerManager runner.Manager
	api           nomad.ExecutorAPI
	defaultJob    nomadApi.Job
}

func (m *NomadEnvironmentManager) CreateOrUpdate(
	id runner.EnvironmentID,
	request dto.ExecutionEnvironmentRequest,
) (bool, error) {
	templateJob, err := m.registerTemplateJob(id,
		request.PrewarmingPoolSize, request.CPULimit, request.MemoryLimit,
		request.Image, request.NetworkAccess, request.ExposedPorts)

	if err != nil {
		return false, err
	}

	created, err := m.runnerManager.CreateOrUpdateEnvironment(id, request.PrewarmingPoolSize, templateJob)
	if err != nil {
		return created, err
	}
	return created, nil
}

func (m *NomadEnvironmentManager) Delete(id string) {

}

func (m *NomadEnvironmentManager) loadExistingEnvironments() error {
	jobs, err := m.api.LoadAllJobs()
	if err != nil {
		return fmt.Errorf("can't load template jobs: %w", err)
	}

	var environmentTemplates, runnerJobs []*nomadApi.Job
	for _, job := range jobs {
		if nomad.IsEnvironmentTemplateID(*job.ID) {
			environmentTemplates = append(environmentTemplates, job)
		} else {
			runnerJobs = append(runnerJobs, job)
		}
	}
	m.recoverJobs(environmentTemplates, m.recoverEnvironmentTemplates)
	m.recoverJobs(runnerJobs, m.recoverRunner)

	err = m.runnerManager.ScaleAllEnvironments()
	if err != nil {
		return fmt.Errorf("can not restore environment scaling: %w", err)
	}
	return nil
}

type jobAdder func(id runner.EnvironmentID, job *nomadApi.Job, configTaskGroup *nomadApi.TaskGroup) error

func (m *NomadEnvironmentManager) recoverEnvironmentTemplates(id runner.EnvironmentID, job *nomadApi.Job,
	configTaskGroup *nomadApi.TaskGroup) error {
	desiredIdleRunnersCount, err := strconv.Atoi(configTaskGroup.Meta[nomad.ConfigMetaPoolSizeKey])
	if err != nil {
		return fmt.Errorf("Couldn't convert pool size to int: %w", err)
	}

	m.runnerManager.RecoverEnvironment(id, job, uint(desiredIdleRunnersCount))
	return nil
}

func (m *NomadEnvironmentManager) recoverRunner(id runner.EnvironmentID, job *nomadApi.Job,
	configTaskGroup *nomadApi.TaskGroup) error {
	isUsed := configTaskGroup.Meta[nomad.ConfigMetaUsedKey] == nomad.ConfigMetaUsedValue
	m.runnerManager.RecoverRunner(id, job, isUsed)
	return nil
}

func (m *NomadEnvironmentManager) recoverJobs(jobs []*nomadApi.Job, onJob jobAdder) {
	for _, job := range jobs {
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
		environmentID, err := runner.NewEnvironmentID(configTaskGroup.Meta[nomad.ConfigMetaEnvironmentKey])
		if err != nil {
			jobLogger.WithField("environmentID", configTaskGroup.Meta[nomad.ConfigMetaEnvironmentKey]).
				WithError(err).
				Error("Couldn't convert environment id of template job to int")
			continue
		}
		err = onJob(environmentID, job, configTaskGroup)
		if err != nil {
			jobLogger.WithError(err).Info("Could not recover job.")
			continue
		}
	}
}
