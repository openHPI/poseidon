package nomad

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"net/url"
)

// ExecutorApi provides access to an container orchestration solution
type ExecutorApi interface {
	// LoadJobList loads the list of jobs from the Nomad api.
	LoadJobList() (list []*nomadApi.JobListStub, err error)
	// GetJobScale returns the scale of the passed job.
	GetJobScale(jobId string) (jobScale int, err error)
	// SetJobScaling sets the scaling count of the passed job to Nomad.
	SetJobScaling(jobId string, count int, reason string) (err error)

	// DeleteRunner deletes the runner with the given Id.
	DeleteRunner(runnerId string) (err error)

	// LoadAvailableRunners loads all allocations of the specified job which are running and not about to get stopped.
	LoadAvailableRunners(jobId string) (runnerIds []string, err error)
}

// ApiClient provides access to the Nomad functionality
type ApiClient struct {
	client *nomadApi.Client
}

// New creates a new api client.
// One client is usually sufficient for the complete runtime of the API.
func New(nomadURL *url.URL) (ExecutorApi, error) {
	client := &ApiClient{}
	err := client.init(nomadURL)
	return client, err
}

// init prepares an apiClient to be able to communicate to a provided Nomad API.
func (apiClient *ApiClient) init(nomadURL *url.URL) (err error) {
	apiClient.client, err = nomadApi.NewClient(&nomadApi.Config{
		Address:   nomadURL.String(),
		TLSConfig: &nomadApi.TLSConfig{},
	})
	return err
}

func (apiClient *ApiClient) LoadJobList() (list []*nomadApi.JobListStub, err error) {
	list, _, err = apiClient.client.Jobs().List(nil)
	return
}

func (apiClient *ApiClient) GetJobScale(jobId string) (jobScale int, err error) {
	status, _, err := apiClient.client.Jobs().ScaleStatus(jobId, nil)
	if err != nil {
		return
	}
	// ToDo: Consider counting also the placed and desired allocations
	jobScale = status.TaskGroups[jobId].Running
	return
}

func (apiClient *ApiClient) SetJobScaling(jobId string, count int, reason string) (err error) {
	_, _, err = apiClient.client.Jobs().Scale(jobId, jobId, &count, reason, false, nil, nil)
	return
}

func (apiClient *ApiClient) LoadAvailableRunners(jobId string) (runnerIds []string, err error) {
	list, _, err := apiClient.client.Jobs().Allocations(jobId, true, nil)
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

func (apiClient *ApiClient) DeleteRunner(runnerId string) (err error) {
	allocation, _, err := apiClient.client.Allocations().Info(runnerId, nil)
	if err != nil {
		return
	}
	_, err = apiClient.client.Allocations().Stop(allocation, nil)
	return err
}
