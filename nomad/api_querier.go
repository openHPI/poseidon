package nomad

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"net/url"
)

// apiQuerier provides access to the Nomad functionality.
type apiQuerier interface {
	// init prepares an apiClient to be able to communicate to a provided Nomad API.
	init(nomadURL *url.URL) (err error)

	// LoadJobList loads the list of jobs from the Nomad API.
	LoadJobList() (list []*nomadApi.JobListStub, err error)

	// JobScale returns the scale of the passed job.
	JobScale(jobId string) (jobScale int, err error)

	// SetJobScaling sets the scaling count of the passed job to Nomad.
	SetJobScale(jobId string, count int, reason string) (err error)

	// DeleteRunner deletes the runner with the given Id.
	DeleteRunner(runnerId string) (err error)

	// loadRunners loads all allocations of the specified job.
	loadRunners(jobId string) (allocationListStub []*nomadApi.AllocationListStub, err error)
}

// nomadApiClient implements the nomadApiQuerier interface and provides access to a real Nomad API.
type nomadApiClient struct {
	client *nomadApi.Client
}

func (nc *nomadApiClient) init(nomadURL *url.URL) (err error) {
	nc.client, err = nomadApi.NewClient(&nomadApi.Config{
		Address:   nomadURL.String(),
		TLSConfig: &nomadApi.TLSConfig{},
	})
	return err
}

func (nc *nomadApiClient) LoadJobList() (list []*nomadApi.JobListStub, err error) {
	list, _, err = nc.client.Jobs().List(nil)
	return
}

func (nc *nomadApiClient) JobScale(jobId string) (jobScale int, err error) {
	status, _, err := nc.client.Jobs().ScaleStatus(jobId, nil)
	if err != nil {
		return
	}
	// ToDo: Consider counting also the placed and desired allocations
	jobScale = status.TaskGroups[jobId].Running
	return
}

func (nc *nomadApiClient) SetJobScale(jobId string, count int, reason string) (err error) {
	_, _, err = nc.client.Jobs().Scale(jobId, jobId, &count, reason, false, nil, nil)
	return
}

func (nc *nomadApiClient) DeleteRunner(runnerId string) (err error) {
	allocation, _, err := nc.client.Allocations().Info(runnerId, nil)
	if err != nil {
		return
	}
	_, err = nc.client.Allocations().Stop(allocation, nil)
	return err
}

func (nc *nomadApiClient) loadRunners(jobId string) (allocationListStub []*nomadApi.AllocationListStub, err error) {
	allocationListStub, _, err = nc.client.Jobs().Allocations(jobId, true, nil)
	return
}
