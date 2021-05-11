package nomad

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"net/url"
)

// ExecutorApi provides access to a container orchestration solution.
type ExecutorApi interface {
	apiQuerier

	// LoadAvailableRunners loads all allocations of the specified job which are running and not about to get stopped.
	LoadAvailableRunners(jobId string) (runnerIds []string, err error)
}

// ApiClient implements the ExecutorApi interface and can be used to perform different operations on the real Executor API and its return values.
type ApiClient struct {
	apiQuerier
}

// NewExecutorApi creates a new api client.
// One client is usually sufficient for the complete runtime of the API.
func NewExecutorApi(nomadURL *url.URL) (ExecutorApi, error) {
	client := &ApiClient{apiQuerier: &nomadApiClient{}}
	err := client.init(nomadURL)
	return client, err
}

func (c *ApiClient) LoadAvailableRunners(jobId string) (runnerIds []string, err error) {
	list, err := c.loadRunners(jobId)
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
