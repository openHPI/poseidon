package environment

import (
	nomadApi "github.com/hashicorp/nomad/api"
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
		id string,
		request dto.ExecutionEnvironmentRequest,
	) (bool, error)

	// Delete removes the execution environment with the given id from the executor.
	Delete(id string)
}

func NewNomadEnvironmentManager(runnerManager runner.Manager, apiClient nomad.ExecutorAPI) *NomadEnvironmentManager {
	environmentManager := &NomadEnvironmentManager{runnerManager, apiClient, *parseJob(defaultJobHCL)}
	return environmentManager
}

type NomadEnvironmentManager struct {
	runnerManager runner.Manager
	api           nomad.ExecutorAPI
	defaultJob    nomadApi.Job
}

func (m *NomadEnvironmentManager) CreateOrUpdate(
	id string,
	request dto.ExecutionEnvironmentRequest,
) (bool, error) {
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return false, err
	}
	err = m.registerDefaultJob(id,
		request.PrewarmingPoolSize, request.CPULimit, request.MemoryLimit,
		request.Image, request.NetworkAccess, request.ExposedPorts)

	if err != nil {
		return false, err
	}

	created, err := m.runnerManager.CreateOrUpdateEnvironment(runner.EnvironmentID(idInt), request.PrewarmingPoolSize)
	if err != nil {
		return created, err
	}
	return created, nil
}

func (m *NomadEnvironmentManager) Delete(id string) {

}
