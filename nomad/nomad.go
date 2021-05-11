package nomad

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"net/url"
)

var log = logging.GetLogger("nomad")

// ExecutorApi provides access to an container orchestration solution
type ExecutorApi interface {
	apiQuerier

	// LoadRunners loads all allocations of the specified job which are running and not about to get stopped.
	LoadRunners(jobId string) (runnerIds []string, err error)
}

// ApiClient implements the ExecutorApi interface and can be used to perform different operations on the real Executor API and its return values.
type ApiClient struct {
	apiQuerier
	defaultJob nomadApi.Job
}

// NewExecutorApi creates a new api client.
// One client is usually sufficient for the complete runtime of the API.
func NewExecutorApi(nomadURL *url.URL, nomadNamespace string) (ExecutorApi, error) {
	client := &ApiClient{apiQuerier: &nomadApiClient{}}
	err := client.init(nomadURL, nomadNamespace)
	return client, err
}

// init prepares an apiClient to be able to communicate to a provided Nomad API.
func (apiClient *ApiClient) init(nomadURL *url.URL, nomadNamespace string) (err error) {
	err = apiClient.apiQuerier.init(nomadURL, nomadNamespace)
	if err != nil {
		return err
	}
	apiClient.defaultJob = *parseJob(defaultJobHCL)
	return nil
}

// LoadRunners loads the allocations of the specified job.
func (apiClient *ApiClient) LoadRunners(jobId string) (runnerIds []string, err error) {
	//list, _, err := apiClient.client.Jobs().Allocations(jobId, true, nil)
	list, err := apiClient.loadRunners(jobId)
	if err != nil {
		return nil, err
	}
	for _, stub := range list {
		// only add allocations which are running and not about to be stopped
		if stub.ClientStatus == nomadApi.AllocClientStatusRunning && stub.DesiredStatus == nomadApi.AllocDesiredStatusRun {
			runnerIds = append(runnerIds, stub.ID)
		}
	}
	return
}
