package environment

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
)

var log = logging.GetLogger("environment")

// Manager encapsulates API calls to the executor API for creation and deletion of execution environments.
type Manager interface {
	// Load fetches all already created execution environments from the executor and registers them at the runner manager.
	// It should be called during the startup process (e.g. on creation of the Manager).
	Load()

	// Create creates a new execution environment on the executor.
	Create(
		id string,
		request dto.ExecutionEnvironmentRequest,
	)

	// Delete removes the execution environment with the given id from the executor.
	Delete(id string)
}

func NewNomadEnvironmentManager(runnerManager runner.Manager, apiClient nomad.ExecutorApi) *NomadEnvironmentManager {
	environmentManager := &NomadEnvironmentManager{runnerManager, apiClient, *parseJob(defaultJobHCL)}
	environmentManager.Load()
	return environmentManager
}

type NomadEnvironmentManager struct {
	runnerManager runner.Manager
	api           nomad.ExecutorApi
	defaultJob    nomadApi.Job
}

func (m *NomadEnvironmentManager) Create(
	id string,
	request dto.ExecutionEnvironmentRequest,
) {

}

func (m *NomadEnvironmentManager) Delete(id string) {

}

func (m *NomadEnvironmentManager) Load() {
	// ToDo: remove create default execution environment for debugging purposes
	m.runnerManager.RegisterEnvironment(runner.EnvironmentId(0), "python", 5)
}
