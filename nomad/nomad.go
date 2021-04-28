package nomad

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"net/url"
)

var log = logging.GetLogger("nomad")

// ExecutorApi provides access to an container orchestration solution
type ExecutorApi interface {
	LoadJobList() (list []*nomadApi.JobListStub, err error)
	GetJobScale(jobId string) (jobScale int, err error)
	SetJobScaling(jobId string, count int, reason string) (err error)
	LoadRunners(jobId string) (runnerIds []string, err error)
	CreateDebugJob()
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

// LoadJobList loads the list of jobs from the Nomad api.
func (apiClient *ApiClient) LoadJobList() (list []*nomadApi.JobListStub, err error) {
	list, _, err = apiClient.client.Jobs().List(nil)
	return
}

// CreateDebugJob creates a simple python job in the nomad cluster
func (apiClient *ApiClient) CreateDebugJob() {
	job := nomadApi.NewBatchJob("python", "python", "global", 50)
	job.AddDatacenter("dc1")
	group := nomadApi.NewTaskGroup("python", 5)
	task := nomadApi.NewTask("python", "docker")
	task.SetConfig("image", "openhpi/co_execenv_python:3.8")
	task.SetConfig("command", "sleep")
	task.SetConfig("args", []string{"infinity"})
	group.AddTask(task)
	job.AddTaskGroup(group)
	register, w, err := apiClient.client.Jobs().Register(job, nil)
	log.Printf("response: %+v", register)
	log.Printf("meta: %+v", w)
	if err != nil {
		log.WithError(err).Fatal("Error creating nomad job")
	}
}

// GetJobScale returns the scale of the passed job.
func (apiClient *ApiClient) GetJobScale(jobId string) (jobScale int, err error) {
	status, _, err := apiClient.client.Jobs().ScaleStatus(jobId, nil)
	if err != nil {
		return
	}
	// ToDo: Consider counting also the placed and desired allocations
	jobScale = status.TaskGroups[jobId].Running
	return
}

// SetJobScaling sets the scaling count of the passed job to Nomad.
func (apiClient *ApiClient) SetJobScaling(jobId string, count int, reason string) (err error) {
	_, _, err = apiClient.client.Jobs().Scale(jobId, jobId, &count, reason, false, nil, nil)
	return
}

// LoadRunners loads the allocations of the specified job.
func (apiClient *ApiClient) LoadRunners(jobId string) (runnerIds []string, err error) {
	list, _, err := apiClient.client.Jobs().Allocations(jobId, true, nil)
	if err != nil {
		return nil, err
	}
	for _, stub := range list {
		runnerIds = append(runnerIds, stub.ID)
	}
	return
}
