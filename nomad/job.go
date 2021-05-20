package nomad

import (
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
)

const (
	TaskGroupNameFormat = "%s-group"
	TaskName            = "default-task"
)

// LoadJobList loads the list of jobs from the Nomad api.
func (nc *nomadApiClient) LoadJobList() (list []*nomadApi.JobListStub, err error) {
	list, _, err = nc.client.Jobs().List(nil)
	return
}

// JobScale returns the scale of the passed job.
func (nc *nomadApiClient) JobScale(jobId string) (jobScale int, err error) {
	status, _, err := nc.client.Jobs().ScaleStatus(jobId, nil)
	if err != nil {
		return
	}
	// ToDo: Consider counting also the placed and desired allocations
	jobScale = status.TaskGroups[fmt.Sprintf(TaskGroupNameFormat, jobId)].Running
	return
}

// SetJobScale sets the scaling count of the passed job to Nomad.
func (nc *nomadApiClient) SetJobScale(jobId string, count int, reason string) (err error) {
	_, _, err = nc.client.Jobs().Scale(jobId, fmt.Sprintf(TaskGroupNameFormat, jobId), &count, reason, false, nil, nil)
	return
}
